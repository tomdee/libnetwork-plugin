package driver

import (
	"context"
	"log"
	"net"

	"github.com/pkg/errors"
	libcalicoErrors "github.com/projectcalico/libcalico-go/lib/errors"

	"github.com/docker/go-plugins-helpers/network"

	dockerClient "github.com/docker/engine-api/client"
	"github.com/projectcalico/libcalico-go/lib/api"
	datastoreClient "github.com/projectcalico/libcalico-go/lib/client"
	caliconet "github.com/projectcalico/libcalico-go/lib/net"

	"github.com/projectcalico/libnetwork-plugin/datastore"
	logutils "github.com/projectcalico/libnetwork-plugin/utils/log"
	"github.com/projectcalico/libnetwork-plugin/utils/netns"
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

// The Calico network driver must be used with Calico IPAM and support IPv4 only.
// TODO - do we really need the NetworkDriverMetadata struct
// TODO - can the driver be called just calico (and not calico-net)

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

	//TODO (Tom) - Make this check "IPv4Data":[{"AddressSpace":"CalicoGlobalAddressSpace"
	// If the AddressSpace isn't CalicoGlobalAddressSpace then return an error
	// This is to ensure that we can only use Calico IPAM

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
		err = errors.Wrap(err, "Hostname fetching error")
		return nil, err
	}

	d.logger.Printf("Creating endpoint %v\n", request.EndpointID)
	if request.Interface.Address == "" {
		return nil, errors.New("No address assigned for endpoint")
	}

	var addresses []caliconet.IPNet
	if request.Interface.Address != "" {
		// Parse the address this function was passed. Ignore the subnet - Calico always uses /32 (for IPv4)
		ip4, _, _ := net.ParseCIDR(request.Interface.Address)
		d.logger.Printf("Parsed IP %v from (%v) \n", ip4, request.Interface.Address)
		if ip4 != nil {
			addresses = append(addresses, caliconet.IPNet{net.IPNet{IP: ip4, Mask: net.CIDRMask(32, 32)}})
		} else {
			//TODO - error handling?
		}
	}

	endpoint := api.NewWorkloadEndpoint()
	endpoint.Metadata.Node = hostname
	endpoint.Metadata.Orchestrator = d.metadata.orchestratorID
	endpoint.Metadata.Workload = d.metadata.containerName
	endpoint.Metadata.Name = request.EndpointID
	endpoint.Spec.InterfaceName = "cali" + request.EndpointID[:min(11, len(request.EndpointID))]
	mac, _ := net.ParseMAC(d.metadata.fixedMac)
	endpoint.Spec.MAC = caliconet.MAC{HardwareAddr: mac}
	endpoint.Spec.IPNetworks = append(endpoint.Spec.IPNetworks, addresses...)

	//Use the Docker API to fetch the network name (so we don't have to use an ID everywhere)
	dockerCli, err := dockerClient.NewEnvClient()
	if err != nil {
		err = errors.Wrap(err, "Error while attempting to instantiate docker client from env")
		return nil, err
	}
	networkData, err := dockerCli.NetworkInspect(context.Background(), request.NetworkID)
	if err != nil {
		err = errors.Wrapf(err, "Network %v inspection error", request.NetworkID)
		return nil, err
	}

	// Now that we know the network name, set it on the endpoint.
	endpoint.Spec.Profiles = append(endpoint.Spec.Profiles, networkData.Name)

	// Check if the profile already exists.
	exists := true
	if _, err = d.client.Profiles().Get(api.ProfileMetadata{Name: networkData.Name}); err != nil {
		_, ok := err.(libcalicoErrors.ErrorResourceDoesNotExist)
		if ok {
			exists = false
		} else {
			// TODO - A genuine error - handle it...
			return nil, err
		}
	}

	// If a profile for the network name doesn't exist then it needs to be created.
	if !exists {
		profile := api.NewProfile()
		profile.Metadata.Name = networkData.Name
		profile.Spec.Tags = []string{networkData.Name}
		profile.Spec.EgressRules = []api.Rule{{Action: "allow"}}
		profile.Spec.IngressRules = []api.Rule{{Action: "allow", Source: api.EntityRule{Tag: networkData.Name}}}
		if _, err := d.client.Profiles().Create(profile); err != nil {
			log.Println(err)
			return nil, err
		}
	}

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
		err = errors.Wrap(err, "Hostname fetching error")
		return err
	}

	logutils.JSONMessage(d.logger, "DeleteEndpoint JSON=%v", request)
	d.logger.Printf("Removing endpoint %v\n", request.EndpointID)

	endpoint := api.NewWorkloadEndpoint()
	endpoint.Metadata.Node = hostname
	endpoint.Metadata.Orchestrator = d.metadata.orchestratorID
	endpoint.Metadata.Workload = d.metadata.containerName
	endpoint.Metadata.Name = request.EndpointID

	// TODO just create a Metadata directly, like this.
	//if err := api.WorkloadEndpoints().Delete(api.WorkloadEndpointMetadata{
	//	Name:         request.EndpointID,
	//	Node:         hostname,
	//	Orchestrator: d.metadata.orchestratorID,
	//	Workload:     d.metadata.containerName}); err != nil {
	//	return err
	//}

	if err = d.client.WorkloadEndpoints().Delete(endpoint.Metadata); err != nil {
		err = errors.Wrapf(err, "Endpoint removal error, data: %+v", endpoint.Metadata)
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

	// 1) Set up a veth pair
	// 	The one end will stay in the host network namespace - named caliXXXXX
	//	THe other end is given a temporary name. It's moved into the final network namespace by libnetwork itself.
	var err error
	hostInterfaceName := "cali" + request.EndpointID[:min(11, len(request.EndpointID))]
	tempInterfaceName := "temp" + request.EndpointID[:min(11, len(request.EndpointID))]

	if err = netns.CreateVeth(hostInterfaceName, tempInterfaceName); err != nil {
		err = errors.Wrapf(
			err, "Veth creation error, hostInterfaceName=%v, tempInterfaceName=%v",
			hostInterfaceName, tempInterfaceName)
		d.logger.Println(err)
		return nil, err
	}

	// libnetwork doesn't set the MAC address properly, so set it here.
	if err = netns.SetVethMac(tempInterfaceName, d.metadata.fixedMac); err != nil {
		d.logger.Printf("Veth mac setting for %v failed, removing veth for %v\n", tempInterfaceName, hostInterfaceName)
		_, err = netns.RemoveVeth(hostInterfaceName)
		err = errors.Wrapf(err, "Veth removing for %v error", hostInterfaceName)
		d.logger.Println(err)
		return nil, err
	}

	resp := &network.JoinResponse{
		InterfaceName: network.InterfaceName{
			SrcName:   tempInterfaceName,
			DstPrefix: IFPrefix,
		},
	}

	// TODO - what should it do
	//        if gateway_ip4:
	//   json_response["Gateway"] = DUMMY_IPV4_NEXTHOP
	//   static_routes.append({
	//       "Destination": DUMMY_IPV4_NEXTHOP + "/32",
	//       "RouteType": 1,  # 1 = CONNECTED
	//       "NextHop": ""
	//   })

	//endpoints, err := d.client.WorkloadEndpoints().List(api.WorkloadEndpointMetadata{})
	//if err != nil {
	//	err = errors.Wrap(err, "Workload endpoints listing error")
	//	d.logger.Println(err)
	//	return nil, err
	//}
	//var useV4, useV6 bool
	//
	//for _, endpoint := range endpoints.Items {
	//	for _, ipNetwork := range endpoint.Spec.IPNetworks {
	//		if ipNetwork.IPNet.IP.To4() != nil {
	//			useV4 = true
	//		} else {
	//			useV6 = true
	//		}
	//	}
	//}

	// One of the network gateway addresses indicate that we are using
	// Calico IPAM driver.  In this case we setup routes using the gateways
	// configured on the endpoint (which will be our host IPs).
	d.logger.Println("Using Calico IPAM driver, configure gateway and " +
		"static routes to the host")

	//if useV4 {
	resp.Gateway = d.metadata.DummyIpV4Nexthop
	resp.StaticRoutes = append(resp.StaticRoutes, &network.StaticRoute{
		Destination: d.metadata.DummyIpV4Nexthop + "/32",
		RouteType:   1, // 1 = CONNECTED
		NextHop:     "",
	})
	//}
	//if useV6 {
	//	// Here, we'll report the link local address of the host's cali interface to libnetwork
	//	// as our IPv6 gateway. IPv6 link local addresses are automatically assigned to interfaces
	//	// when they are brought up. Unfortunately, the container link must be up as well. So
	//	// bring it up now
	//	if err = netns.BringUpInterface(tempInterfaceName); err != nil {
	//		err = errors.Wrapf(err, "Error while bringing up interface %v", tempInterfaceName)
	//		return nil, err
	//	}
	//	// Then extract the link local address that was just assigned to our host's interface
	//	nextHop6, err := netns.GetIPv6LinkLocal(hostInterfaceName)
	//	if err != nil {
	//		err = errors.Wrapf(err, "Error while getting ipv6 local link for %v", hostInterfaceName)
	//		return nil, err
	//	}
	//	resp.GatewayIPv6 = string(nextHop6)
	//	var destination *caliconet.IPNet
	//	if _, destination, err = caliconet.ParseCIDR(string(nextHop6)); err != nil {
	//		err = errors.Wrapf(err, "Error while parsing CIDR out of %v", nextHop6)
	//		return nil, err
	//	}
	//	resp.StaticRoutes = append(resp.StaticRoutes, &network.StaticRoute{
	//		Destination: fmt.Sprintf("%v", destination),
	//		RouteType:   1,
	//		NextHop:     "",
	//	})
	//}

	logutils.JSONMessage(d.logger, "Join Response JSON=%v", resp)

	return resp, nil
}

func (d NetworkDriver) Leave(request *network.LeaveRequest) error {
	logutils.JSONMessage(d.logger, "Leave response JSON=%v", request)
	caliName := "cali" + request.EndpointID[:min(11, len(request.EndpointID))]
	_, err := netns.RemoveVeth(caliName)
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
