package driver

import (
	"errors"
	"log"
	"net"

	"github.com/docker/go-plugins-helpers/network"

	"github.com/tigera/libcalico-go/lib/api"
	datastoreClient "github.com/tigera/libcalico-go/lib/client"
	caliconet "github.com/tigera/libcalico-go/lib/net"

	"github.com/libnetwork-plugin/libnetwork-go/datastore"
	logutils "github.com/libnetwork-plugin/libnetwork-go/utils/log"
	"github.com/libnetwork-plugin/libnetwork-go/utils/netns"
	networkutils "github.com/libnetwork-plugin/libnetwork-go/utils/network"
	osutils "github.com/libnetwork-plugin/libnetwork-go/utils/os"
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
		client: client,
		logger: logger,

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
	profile, err := d.client.Profiles().Create(profile)
	if err != nil {
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
			optionsError := errors.New("Invalid options")
			var (
				options          map[string]bool
				optionsInterface interface{}
				ok               bool

				ipip, masquerade bool
			)

			if optionsInterface, ok = request.Options["com.docker.network.generic"]; !ok {
				return optionsError
			}

			if options, ok = optionsInterface.(map[string]bool); !ok {
				return optionsError
			}

			if ipip, ok = options["ipip"]; !ok {
				return errors.New("ipip option is not provided")
			}

			if masquerade, ok = options["nat-outgoing"]; !ok {
				return errors.New("nat-outgoing option is not provided")
			}

			_, err := d.client.Pools().Create(&api.Pool{
				Metadata: api.PoolMetadata{CIDR: *pool},
				Spec: api.PoolSpec{
					IPIP:        &api.IPIPConfiguration{Enabled: ipip},
					NATOutgoing: masquerade,
				},
			})
			if err != nil {
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
	return err
}

func (d NetworkDriver) DeleteNetwork(request *network.DeleteNetworkRequest) error {
	logutils.JSONMessage(d.logger, "DeleteNetwork JSON=%v", request)
	var err error

	profile := api.NewProfile()
	profile.Metadata.Name = request.NetworkID
	if err = d.client.Profiles().Delete(profile.Metadata); err != nil {
		d.logger.Println(err)
		return err
	}
	d.logger.Printf("Removed profile %v\n", request.NetworkID)

	var networkData *datastore.Network
	if networkData, err = d.datastore.GetNetwork(request.NetworkID); err != nil {
		return err
	}

	IPData := map[string][]*network.IPAMData{
		IPv4: networkData.IPv4Data,
		IPv6: networkData.IPv6Data,
	}

	var gateway, pool *caliconet.IPNet

	for version, data := range IPData {
		gateway, pool, err = networkutils.GetGatewayPool(d.logger, data, version)
		if err != nil {
			continue
		}
		if gateway != nil && !networkutils.IsUsingCalicoIpam(gateway, d.metadata.gatewayCIDRV4, d.metadata.gatewayCIDRV6) {
			if err = d.client.Pools().Delete(api.PoolMetadata{CIDR: *pool}); err != nil {
				log.Println(err)
				return err
			}
			d.logger.Printf("Removed pool %v\n", pool)
		}
	}

	err = d.datastore.RemoveNetwork(request.NetworkID)
	return err
}

func (d NetworkDriver) CreateEndpoint(request *network.CreateEndpointRequest) (*network.CreateEndpointResponse, error) {
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

	if _, addressIP4, err := caliconet.ParseCIDR(request.Interface.Address); err == nil {
		addresses = append(addresses, *addressIP4)
	}
	if _, addressIP6, err := caliconet.ParseCIDR(request.Interface.AddressIPv6); err == nil {
		addresses = append(addresses, *addressIP6)
	}

	endpoint := api.NewWorkloadEndpoint()
	endpoint.Metadata.Hostname = hostname
	endpoint.Metadata.OrchestratorID = d.metadata.orchestratorID
	endpoint.Metadata.WorkloadID = d.metadata.containerName
	endpoint.Metadata.Name = request.EndpointID
	endpoint.Spec.MAC = caliconet.MAC{HardwareAddr: net.HardwareAddr(d.metadata.fixedMac)}

	endpoint.Spec.Profiles = append(endpoint.Spec.Profiles, request.NetworkID)
	endpoint.Spec.IPNetworks = append(endpoint.Spec.IPNetworks, addresses...)

	_, err = d.client.WorkloadEndpoints().Create(endpoint)
	if err != nil {
		return nil, err
	}

	logutils.JSONMessage(d.logger, "CreateEndpoint JSON=%v", request)

	return &network.CreateEndpointResponse{
		Interface: &network.EndpointInterface{
			Address:     request.Interface.Address,
			AddressIPv6: request.Interface.AddressIPv6,
			MacAddress:  string(d.metadata.fixedMac),
		},
	}, nil
}

func (d NetworkDriver) DeleteEndpoint(request *network.DeleteEndpointRequest) error {
	hostname, err := osutils.GetHostname()
	if err != nil {
		return err
	}

	logutils.JSONMessage(d.logger, "DeleteEndpoint JSON=%v", request)
	d.logger.Printf("Removing endpoint %v\n", request.EndpointID)

	endpoint := api.NewWorkloadEndpoint()
	endpoint.Metadata.Hostname = hostname
	endpoint.Metadata.OrchestratorID = d.metadata.orchestratorID
	endpoint.Metadata.WorkloadID = d.metadata.containerName
	endpoint.Metadata.Name = request.EndpointID

	if err = d.client.WorkloadEndpoints().Delete(endpoint.Metadata); err != nil {
		log.Println(err)
		return err
	}

	logutils.JSONMessage(d.logger, "DeleteEndpoint response JSON=%v", map[string]string{})

	return err
}

func (d NetworkDriver) EndpointInfo(*network.InfoRequest) (*network.InfoResponse, error) {
	return nil, nil
}

func (d NetworkDriver) Join(request *network.JoinRequest) (*network.JoinResponse, error) {
	var hostInterfaceName, tempInterfaceName string
	var err error
	if hostInterfaceName, err = networkutils.GenerateCaliInterfaceName(d.metadata.ifPrefix, request.EndpointID); err != nil {
		d.logger.Println(err)
		return nil, err
	}
	if tempInterfaceName, err = networkutils.GenerateCaliInterfaceName("tmp", request.EndpointID); err != nil {
		d.logger.Println(err)
		return nil, err
	}

	if err = netns.CreateVeth(hostInterfaceName, tempInterfaceName); err != nil {
		d.logger.Println(err)
		return nil, err
	}

	if err = netns.SetVethMac(tempInterfaceName, d.metadata.fixedMac); err != nil {
		_, err = netns.RemoveVeth(hostInterfaceName)
		d.logger.Println(err)
		return nil, err
	}

	var (
		networkData *datastore.Network
		gatewayV4   *caliconet.IPNet
		gatewayV6   *caliconet.IPNet
	)

	if networkData, err = d.datastore.GetNetwork(request.NetworkID); err != nil {
		d.logger.Println(err)
		return nil, err
	}

	if gatewayV4, _, err = networkutils.GetGatewayPool(d.logger, networkData.IPv4Data, IPv4); err != nil {
		d.logger.Println(err)
		return nil, err
	}
	if gatewayV6, _, err = networkutils.GetGatewayPool(d.logger, networkData.IPv6Data, IPv6); err != nil {
		d.logger.Println(err)
		return nil, err
	}

	useV4 := gatewayV4 != nil &&
		networkutils.IsUsingCalicoIpam(gatewayV4, d.metadata.gatewayCIDRV4, d.metadata.gatewayCIDRV6)
	useV6 := gatewayV6 != nil &&
		networkutils.IsUsingCalicoIpam(gatewayV6, d.metadata.gatewayCIDRV4, d.metadata.gatewayCIDRV6)

	resp := &network.JoinResponse{}

	if useV4 || useV6 {
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
		// TODO: to be implemented
	}

	return nil, nil
}

func (d NetworkDriver) Leave(*network.LeaveRequest) error {

	return nil
}

func (d NetworkDriver) DiscoverNew(*network.DiscoveryNotification) error {
	return nil
}

func (d NetworkDriver) DiscoverDelete(*network.DiscoveryNotification) error {
	return nil
}

func (d NetworkDriver) ProgramExternalConnectivity(*network.ProgramExternalConnectivityRequest) error {
	return nil
}

func (d NetworkDriver) RevokeExternalConnectivity(*network.RevokeExternalConnectivityRequest) error {
	return nil
}
