package driver

import "github.com/docker/go-plugins-helpers/network"

type CalicoDriver struct{}

func (d CalicoDriver) GetCapabilities() (*network.CapabilitiesResponse, error) {
	return &network.CapabilitiesResponse{Scope: "local"}, nil
}

func (d CalicoDriver) CreateNetwork(*network.CreateNetworkRequest) error {
	return nil
}

func (d CalicoDriver) DeleteNetwork(*network.DeleteNetworkRequest) error {
	return nil
}

func (d CalicoDriver) CreateEndpoint(*network.CreateEndpointRequest) (*network.CreateEndpointResponse, error) {
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
