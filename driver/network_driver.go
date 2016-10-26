package driver

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/pkg/errors"

	"github.com/docker/go-plugins-helpers/network"

	dockerClient "github.com/docker/engine-api/client"
	"github.com/projectcalico/libcalico-go/lib/api"
	datastoreClient "github.com/projectcalico/libcalico-go/lib/client"
	caliconet "github.com/projectcalico/libcalico-go/lib/net"

	"github.com/projectcalico/libnetwork-plugin/datastore"
	logutils "github.com/projectcalico/libnetwork-plugin/utils/log"
	"github.com/projectcalico/libnetwork-plugin/utils/netns"
	networkutils "github.com/projectcalico/libnetwork-plugin/utils/network"
	osutils "github.com/projectcalico/libnetwork-plugin/utils/os"
)

type NetworkDriverMetadata struct {
	containerName  string
	orchestratorID string
	fixedMac       string

	gatewayCIDRV4 string
	gatewayCIDRV6 string

	ifPrefix string

	DummyIpV4Nexthop string
}

type NetworkDriver struct {
	client    *datastoreClient.Client
	datastore datastore.Datastore
	logger    *log.Logger

	metadata NetworkDriverMetadata
}

func NewNetworkDriver(client *datastoreClient.Client, datastore datastore.Datastore, logger *log.Logger) network.Driver {
	return NetworkDriver{
		client:    client,
		logger:    logger,
		datastore: datastore,

		metadata: NetworkDriverMetadata{

			// The MAC address of the interface in the container is arbitrary, so for
			// simplicity, use a fixed MAC.
			fixedMac: "EE:EE:EE:EE:EE:EE",

			// Orchestrator and container IDs used in our endpoint identification. These
			// are fixed for libnetwork.  Unique endpoint identification is provided by
			// hostname and endpoint ID.
			containerName:  "libnetwork",
			orchestratorID: "libnetwork",

			gatewayCIDRV4: GatewayCIDRV4,
			gatewayCIDRV6: GatewayCIDRV6,

			ifPrefix:         IFPrefix,
			DummyIpV4Nexthop: "169.254.1.1",
		},
	}
}

func (d NetworkDriver) GetCapabilities() (*network.CapabilitiesResponse, error) {
	resp := network.CapabilitiesResponse{Scope: "global"}
	logutils.JSONMessage(d.logger, "GetCapabilities response JSON=%v", resp)
	return &resp, nil
}

func (d NetworkDriver) CreateNetwork(request *network.CreateNetworkRequest) error {
	logutils.JSONMessage(d.logger, "CreateNetwork JSON=%s", request)

	profile := api.NewProfile()
	profile.Metadata.Name = request.NetworkID
	profile.Spec.Tags = []string{request.NetworkID}
	profile.Spec.EgressRules = []api.Rule{{Action: "allow"}}
	profile.Spec.IngressRules = []api.Rule{{Action: "allow", Source: api.EntityRule{Tag: request.NetworkID}}}
	profile, err := d.client.Profiles().Create(profile)
	if err != nil {
		err = errors.Wrapf(err, "Profile creation error, data: %+v", profile)
		d.logger.Println(err)
		return err
	}

	IPData := map[string][]*network.IPAMData{
		IPv4: request.IPv4Data,
		IPv6: request.IPv6Data,
	}

	for version, data := range IPData {
		// Extract the gateway and pool from the network data.  If this
		// indicates that Calico IPAM is not being used, then create a Calico
		// IP pool.
		gateway, pool, err := networkutils.GetGatewayPool(d.logger, data, version)
		if err != nil {
			err = errors.Wrapf(
				err, "Gateway pool getting error, data = %v, version = %v", data, version)
			d.logger.Println(err)
			return err
		}
		if gateway == nil {
			continue
		}
		// If we aren't using Calico IPAM then we need to ensure an IP pool
		// exists.  IPIP and Masquerade options can be included on the network
		// create as additional options.  Note that this IP Pool has ipam=False
		// to ensure it is not used in Calico IPAM assignment.
		if !networkutils.IsUsingCalicoIpam(gateway, d.metadata.gatewayCIDRV4, d.metadata.gatewayCIDRV6) {
			var spec = api.PoolSpec{Disabled: false}

			if optionsInterface, ok := request.Options["com.docker.network.generic"]; ok {
				if options, ok := optionsInterface.(map[string]interface{}); !ok {
					if ipipInterface, ok := options["ipip"]; ok {
						ipip, ok := ipipInterface.(bool)
						if ok {
							spec.IPIP = &api.IPIPConfiguration{Enabled: ipip}
						}
					}
					if masqueradeInterface, ok := options["nat-outgoing"]; !ok {
						masquerade, ok := masqueradeInterface.(bool)
						if ok {
							spec.NATOutgoing = masquerade
						}
					}
				}
			}

			pool := &api.Pool{
				Metadata: api.PoolMetadata{CIDR: *pool},
				Spec:     spec,
			}

			_, err := d.client.Pools().Create(pool)

			if err != nil {
				err = errors.Wrapf(err, "Pool creation error, data = %v", *pool)
				return err
			}
		}
	}

	err = d.datastore.WriteNetwork(request.NetworkID, datastore.Network{
		NetworkID: request.NetworkID,
		Options:   request.Options,
		IPv4Data:  request.IPv4Data,
		IPv6Data:  request.IPv6Data,
	})

	logutils.JSONMessage(d.logger, "CreateNetwork response JSON=%v", map[string]string{})
	return nil
}

func (d NetworkDriver) DeleteNetwork(request *network.DeleteNetworkRequest) error {
	logutils.JSONMessage(d.logger, "DeleteNetwork JSON=%v", request)
	return nil
}

func (d NetworkDriver) CreateEndpoint(request *network.CreateEndpointRequest) (*network.CreateEndpointResponse, error) {
	logutils.JSONMessage(d.logger, "CreateEndpoint JSON=%v", request)
	hostname, err := osutils.GetHostname()
	if err != nil {
		return nil, err
	}

	logutils.JSONMessage(d.logger, "CreateEndpoint JSON=%v", request)
	d.logger.Printf("Creating endpoint %v\n", request.EndpointID)

	var (
		addresses []caliconet.IPNet
	)

	if request.Interface.Address == "" && request.Interface.AddressIPv6 == "" {
		return nil, errors.New("No address assigned for endpoint")
	}

	if request.Interface.Address != "" {
		ip4, _, _ := net.ParseCIDR(request.Interface.Address)
		d.logger.Printf("Parsed IP %v from (%v) \n", ip4, request.Interface.Address)
		if ip4 != nil {
			addresses = append(addresses, caliconet.IPNet{net.IPNet{IP: ip4, Mask: net.CIDRMask(32, 32)}})
		}
	}

	//TODO IPv6 is broken
	if _, addressIP6, err := caliconet.ParseCIDR(request.Interface.AddressIPv6); err == nil {
		addresses = append(addresses, *addressIP6)
	}

	endpoint := api.NewWorkloadEndpoint()
	endpoint.Metadata.Node = hostname
	endpoint.Metadata.Orchestrator = d.metadata.orchestratorID
	endpoint.Metadata.Workload = d.metadata.containerName
	endpoint.Metadata.Name = request.EndpointID
	endpoint.Spec.InterfaceName, _ = networkutils.GenerateCaliInterfaceName(d.metadata.ifPrefix, request.EndpointID)
	mac, _ := net.ParseMAC(d.metadata.fixedMac)
	endpoint.Spec.MAC = caliconet.MAC{HardwareAddr: mac}

	dockerCli, _ := dockerClient.NewEnvClient()
	networkData, _ := dockerCli.NetworkInspect(context.Background(), request.NetworkID)

	var profile *api.Profile

	if profile, err = d.client.Profiles().Get(api.ProfileMetadata{Name: networkData.Name}); err != nil {
		profile = api.NewProfile()
		profile.Metadata.Name = networkData.Name
		profile.Spec.Tags = []string{networkData.Name}
		profile.Spec.EgressRules = []api.Rule{{Action: "allow"}}
		profile.Spec.IngressRules = []api.Rule{{Action: "allow", Source: api.EntityRule{Tag: request.NetworkID}}}
		if _, err := d.client.Profiles().Create(profile); err != nil {
			log.Println(err)
			return nil, err
		}
	}

	endpoint.Spec.Profiles = append(endpoint.Spec.Profiles, request.NetworkID)
	endpoint.Spec.IPNetworks = append(endpoint.Spec.IPNetworks, addresses...)

	_, err = d.client.WorkloadEndpoints().Create(endpoint)
	if err != nil {
		err = errors.Wrapf(err, "Workload endpoints creation error, data: %+v", endpoint)
		return nil, err
	}

	d.logger.Printf("Workload created, data: %+v\n", endpoint)

	response := &network.CreateEndpointResponse{
		Interface: &network.EndpointInterface{
			MacAddress: string(d.metadata.fixedMac),
		},
	}

	logutils.JSONMessage(d.logger, "CreateEndpoint response JSON=%v", response)

	return response, nil
}

func (d NetworkDriver) DeleteEndpoint(request *network.DeleteEndpointRequest) error {
	logutils.JSONMessage(d.logger, "DeleteEndpoint JSON=%v", request)
	hostname, err := osutils.GetHostname()
	if err != nil {
		return err
	}

	logutils.JSONMessage(d.logger, "DeleteEndpoint JSON=%v", request)
	d.logger.Printf("Removing endpoint %v\n", request.EndpointID)

	endpoint := api.NewWorkloadEndpoint()
	endpoint.Metadata.Node = hostname
	endpoint.Metadata.Orchestrator = d.metadata.orchestratorID
	endpoint.Metadata.Workload = d.metadata.containerName
	endpoint.Metadata.Name = request.EndpointID

	if err = d.client.WorkloadEndpoints().Delete(endpoint.Metadata); err != nil {
		log.Println(err)
		return err
	}

	logutils.JSONMessage(d.logger, "DeleteEndpoint response JSON=%v", map[string]string{})

	return err
}

func (d NetworkDriver) EndpointInfo(request *network.InfoRequest) (*network.InfoResponse, error) {
	logutils.JSONMessage(d.logger, "EndpointInfo JSON=%v", request)
	return nil, nil
}

func (d NetworkDriver) Join(request *network.JoinRequest) (*network.JoinResponse, error) {
	logutils.JSONMessage(d.logger, "Join JSON=%v", request)
	var hostInterfaceName, tempInterfaceName string
	var err error
	if hostInterfaceName, err = networkutils.GenerateCaliInterfaceName(
		d.metadata.ifPrefix, request.EndpointID); err != nil {
		err = errors.Wrapf(
			err,
			"Host interface name generation error, ifPrefix = %v, endpoint id = %v",
			d.metadata.ifPrefix, request.EndpointID)
		d.logger.Println(err)
		return nil, err
	}
	if tempInterfaceName, err = networkutils.GenerateCaliInterfaceName(
		"tmp", request.EndpointID); err != nil {
		err = errors.Wrapf(err, "Temporary interface name generation error, endpoint id = %v", request.EndpointID)
		d.logger.Println(err)
		return nil, err
	}

	if err = netns.CreateVeth(hostInterfaceName, tempInterfaceName); err != nil {
		err = errors.Wrapf(
			err, "Veth creation error, hostInterfaceName=%v, tempInterfaceName=%v",
			hostInterfaceName, tempInterfaceName)
		d.logger.Println(err)
		return nil, err
	}

	if err = netns.SetVethMac(tempInterfaceName, d.metadata.fixedMac); err != nil {
		d.logger.Printf("Veth mac setting for %v failed, removing veth for %v\n", tempInterfaceName, hostInterfaceName)
		_, err = netns.RemoveVeth(hostInterfaceName)
		err = errors.Wrapf(err, "Veth removing for %v error", hostInterfaceName)
		d.logger.Println(err)
		return nil, err
	}

	var (
		networkData *datastore.Network
		gatewayV4   *caliconet.IPNet
		gatewayV6   *caliconet.IPNet
	)

	if networkData, err = d.datastore.GetNetwork(request.NetworkID); err != nil {
		err = errors.Wrapf(err, "Error while getting network %v", request.NetworkID)
		d.logger.Printf("Error getting network: %v", err)
		return nil, err
	}

	// Extract relevant data from the Network data.
	if gatewayV4, _, err = networkutils.GetGatewayPool(d.logger, networkData.IPv4Data, IPv4); err != nil {
		err = errors.Wrapf(err, "Error while getting gateway pool for %+v, %v", networkData.IPv4Data, IPv4)
		d.logger.Println(err)
		return nil, err
	}
	if gatewayV6, _, err = networkutils.GetGatewayPool(d.logger, networkData.IPv6Data, IPv6); err != nil {
		err = errors.Wrapf(err, "Error while getting gateway pool for %+v, %v", networkData.IPv6Data, IPv6)
		d.logger.Printf("Error getting gateway pool V6: %v", err)
		return nil, err
	}

	useV4 := gatewayV4 != nil &&
		networkutils.IsUsingCalicoIpam(gatewayV4, d.metadata.gatewayCIDRV4, d.metadata.gatewayCIDRV6)
	useV6 := gatewayV6 != nil &&
		networkutils.IsUsingCalicoIpam(gatewayV6, d.metadata.gatewayCIDRV4, d.metadata.gatewayCIDRV6)

	resp := &network.JoinResponse{
		InterfaceName: network.InterfaceName{
			SrcName:   tempInterfaceName,
			DstPrefix: IFPrefix,
		},
	}

	if useV4 || useV6 {
		// One of the network gateway addresses indicate that we are using
		// Calico IPAM driver.  In this case we setup routes using the gateways
		// configured on the endpoint (which will be our host IPs).
		d.logger.Println("Using Calico IPAM driver, configure gateway and " +
			"static routes to the host")
		if gatewayV4 != nil {
			resp.Gateway = d.metadata.DummyIpV4Nexthop
			resp.StaticRoutes = append(resp.StaticRoutes, &network.StaticRoute{
				Destination: d.metadata.DummyIpV4Nexthop + "/32",
				RouteType:   1,
				NextHop:     "",
			})
		}
		if gatewayV6 != nil {
			// Here, we'll report the link local address of the host's cali interface to libnetwork
			// as our IPv6 gateway. IPv6 link local addresses are automatically assigned to interfaces
			// when they are brought up. Unfortunately, the container link must be up as well. So
			// bring it up now
			if err = netns.BringUpInterface(tempInterfaceName); err != nil {
				err = errors.Wrapf(err, "Error while bringing up interface %v", tempInterfaceName)
				return nil, err
			}
			// Then extract the link local address that was just assigned to our host's interface
			nextHop6, err := netns.GetIPv6LinkLocal(hostInterfaceName)
			if err != nil {
				err = errors.Wrapf(err, "Error while getting ipv6 local link for %v", hostInterfaceName)
				return nil, err
			}
			resp.GatewayIPv6 = string(nextHop6)
			var destination *caliconet.IPNet
			if _, destination, err = caliconet.ParseCIDR(string(nextHop6)); err != nil {
				err = errors.Wrapf(err, "Error while parsing CIDR out of %v", nextHop6)
				return nil, err
			}
			resp.StaticRoutes = append(resp.StaticRoutes, &network.StaticRoute{
				Destination: fmt.Sprintf("%v", destination),
				RouteType:   1,
				NextHop:     "",
			})
		}
	} else {
		// We are not using Calico IPAM driver, so configure blank gateways to
		// set up auto-gateway behavior.
		// Default empty values for Gateway and GatewayIPv6 are used.
		d.logger.Println("Not using Calico IPAM driver")
	}

	logutils.JSONMessage(d.logger, "Join Response JSON=%v", resp)

	return resp, nil
}

func (d NetworkDriver) Leave(request *network.LeaveRequest) error {
	logutils.JSONMessage(d.logger, "Leave response JSON=%v", request)
	caliName, err := networkutils.GenerateCaliInterfaceName(d.metadata.ifPrefix, request.EndpointID)
	if err != nil {
		d.logger.Println(err)
		return err
	}
	_, err = netns.RemoveVeth(caliName)
	return err
}

func (d NetworkDriver) DiscoverNew(request *network.DiscoveryNotification) error {
	logutils.JSONMessage(d.logger, "DiscoverNew JSON=%v", request)
	d.logger.Println("DiscoverNew response JSON={}")
	return nil
}

func (d NetworkDriver) DiscoverDelete(request *network.DiscoveryNotification) error {
	logutils.JSONMessage(d.logger, "DiscoverNew JSON=%v", request)
	d.logger.Println("DiscoverDelete response JSON={}")
	return nil
}

func (d NetworkDriver) ProgramExternalConnectivity(*network.ProgramExternalConnectivityRequest) error {
	return nil
}

func (d NetworkDriver) RevokeExternalConnectivity(*network.RevokeExternalConnectivityRequest) error {
	return nil
}
