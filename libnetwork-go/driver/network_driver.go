package driver

import (
	"errors"
	"log"

	"github.com/docker/go-plugins-helpers/network"
	logutils "github.com/libnetwork-plugin/libnetwork-go/utils/log"
	"github.com/tigera/libcalico-go/lib/api"
	datastoreClient "github.com/tigera/libcalico-go/lib/client"
)

const (
	// The MAC address of the interface in the container is arbitrary, so for
	// simplicity, use a fixed MAC.
	FIXED_MAC = "EE:EE:EE:EE:EE:EE"

	// Orchestrator and container IDs used in our endpoint identification. These
	// are fixed for libnetwork.  Unique endpoint identification is provided by
	// hostname and endpoint ID.
	CONTAINER_NAME  = "libnetwork"
	ORCHESTRATOR_ID = "libnetwork"
)

type CalicoDriver struct {
	client *datastoreClient.Client
	logger *log.Logger
}

func (d CalicoDriver) GetCapabilities() (*network.CapabilitiesResponse, error) {
	return &network.CapabilitiesResponse{Scope: "global"}, nil
}

func (d CalicoDriver) CreateNetwork(*network.CreateNetworkRequest) error {
	return nil
}

func (d CalicoDriver) DeleteNetwork(*network.DeleteNetworkRequest) error {
	return nil
}

func (d CalicoDriver) CreateEndpoint(request *network.CreateEndpointRequest) (*network.CreateEndpointResponse, error) {
	logutils.JSONMessage(d.logger, "CreateEndpoint JSON=%v", request)
	d.logger.Printf("Creating endpoint %v\n", request.EndpointID)

	if request.Interface.Address == "" && request.Interface.AddressIPv6 == "" {
		return nil, errors.New("No address assigned for endpoint")
	}

	endpoint := api.NewHostEndpoint()

	d.client.HostEndpoints().Create(endpoint)

	return nil, nil
}

func (d CalicoDriver) DeleteEndpoint(*network.DeleteEndpointRequest) error {
	return nil
}

func (d CalicoDriver) EndpointInfo(*network.InfoRequest) (*network.InfoResponse, error) {
	return nil, nil
}

func (d CalicoDriver) Join(*network.JoinRequest) (*network.JoinResponse, error) {
	return nil, nil
}

func (d CalicoDriver) Leave(*network.LeaveRequest) error {
	return nil
}

func (d CalicoDriver) DiscoverNew(*network.DiscoveryNotification) error {
	return nil
}

func (d CalicoDriver) DiscoverDelete(*network.DiscoveryNotification) error {
	return nil
}

func (d CalicoDriver) ProgramExternalConnectivity(*network.ProgramExternalConnectivityRequest) error {
	return nil
}

func (d CalicoDriver) RevokeExternalConnectivity(*network.RevokeExternalConnectivityRequest) error {
	return nil
}
