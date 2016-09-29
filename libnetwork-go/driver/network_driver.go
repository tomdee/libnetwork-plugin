package driver

import (
	"errors"
	"log"
	"net"

	"github.com/docker/go-plugins-helpers/network"

	"github.com/tigera/libcalico-go/lib/api"
	datastoreClient "github.com/tigera/libcalico-go/lib/client"
	caliconet "github.com/tigera/libcalico-go/lib/net"

	logutils "github.com/libnetwork-plugin/libnetwork-go/utils/log"
	networkutils "github.com/libnetwork-plugin/libnetwork-go/utils/network"
	osutils "github.com/libnetwork-plugin/libnetwork-go/utils/os"
)

type NetworkDriver struct {
	client *datastoreClient.Client
	logger *log.Logger

	containerName  string
	orchestratorID string
	fixedMac       net.HardwareAddr

	gatewayCIDRV4 string
	gatewayCIDRV6 string
}

func NewNetworkDriver(client *datastoreClient.Client, logger *log.Logger) network.Driver {
	return NetworkDriver{
		client: client,
		logger: logger,
		// The MAC address of the interface in the container is arbitrary, so for
		// simplicity, use a fixed MAC.
		fixedMac: net.HardwareAddr("EE:EE:EE:EE:EE:EE"),

		// Orchestrator and container IDs used in our endpoint identification. These
		// are fixed for libnetwork.  Unique endpoint identification is provided by
		// hostname and endpoint ID.
		containerName:  "libnetwork",
		orchestratorID: "libnetwork",

		gatewayCIDRV4: GatewayCIDRV4,
		gatewayCIDRV6: GatewayCIDRV6,
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
		"IPv4": request.IPv4Data,
		"IPv6": request.IPv6Data,
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
		if !networkutils.IsUsingCalicoIpam(gateway, d.gatewayCIDRV4, d.gatewayCIDRV6) {
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
	// TODO: to be implemented
	return nil
}

func (d NetworkDriver) DeleteNetwork(*network.DeleteNetworkRequest) error {
	return nil
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
	endpoint.Metadata.OrchestratorID = d.orchestratorID
	endpoint.Metadata.WorkloadID = d.containerName
	endpoint.Metadata.Name = request.EndpointID
	endpoint.Spec.MAC = caliconet.MAC{HardwareAddr: d.fixedMac}

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
			MacAddress:  string(d.fixedMac),
		},
	}, nil
}

func (d NetworkDriver) DeleteEndpoint(*network.DeleteEndpointRequest) error {
	return nil
}

func (d NetworkDriver) EndpointInfo(*network.InfoRequest) (*network.InfoResponse, error) {
	return nil, nil
}

func (d NetworkDriver) Join(*network.JoinRequest) (*network.JoinResponse, error) {
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
