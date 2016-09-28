package driver

import (
	"errors"
	"log"
	"net"

	"github.com/docker/go-plugins-helpers/network"
	logutils "github.com/libnetwork-plugin/libnetwork-go/utils/log"
	osutils "github.com/libnetwork-plugin/libnetwork-go/utils/os"
	"github.com/tigera/libcalico-go/lib/api"
	datastoreClient "github.com/tigera/libcalico-go/lib/client"
	caliconet "github.com/tigera/libcalico-go/lib/net"
)

type NetworkDriver struct {
	client         *datastoreClient.Client
	logger         *log.Logger
	containerName  string
	orchestratorID string
	fixedMac       net.HardwareAddr
}

func NewNetworkDriver(client *datastoreClient.Client, logger *log.Logger) NetworkDriver {
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
	}
}

func (d NetworkDriver) GetCapabilities() (*network.CapabilitiesResponse, error) {
	return &network.CapabilitiesResponse{Scope: "global"}, nil
}

func (d NetworkDriver) CreateNetwork(*network.CreateNetworkRequest) error {
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
