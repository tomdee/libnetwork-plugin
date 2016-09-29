package network

import (
	"bytes"
	"errors"
	"fmt"
	"log"

	"github.com/docker/go-plugins-helpers/network"
	caliconet "github.com/tigera/libcalico-go/lib/net"
)

func GetGatewayPool(logger *log.Logger, data []*network.IPAMData, ipVersion string) (*caliconet.IPNet, *caliconet.IPNet, error) {
	if len(data) == 0 {
		return nil, nil, nil
	} else if len(data) > 1 {
		err := fmt.Errorf("Unsupported: multiple Gateways defined for %v", ipVersion)
		logger.Println(err)
		return nil, nil, err
	}
	if data[0].Gateway == "" || data[0].Pool == "" {
		return nil, nil, nil
	}
	_, pool, poolErr := caliconet.ParseCIDR(data[0].Pool)
	_, gateway, gatewayErr := caliconet.ParseCIDR(data[0].Gateway)
	if poolErr != nil || gatewayErr != nil {
		return nil, nil, nil
	}
	return gateway, pool, nil
}

func IsUsingCalicoIpam(gateway *caliconet.IPNet, gatewayCIDRV4, gatewayCIDRV6 string) bool {
	var ipNets []*caliconet.IPNet
	if _, v4, err := caliconet.ParseCIDR(gatewayCIDRV4); err == nil {
		ipNets = append(ipNets, v4)
	}
	if _, v6, err := caliconet.ParseCIDR(gatewayCIDRV6); err == nil {
		ipNets = append(ipNets, v6)
	}

	for _, ipNet := range ipNets {

		if bytes.Compare(ipNet.IPNet.IP, gateway.IPNet.IP) == 0 &&
			bytes.Compare(ipNet.IPNet.Mask, gateway.IPNet.Mask) == 0 {
			return true
		}
	}

	return false
}

// GenerateCaliInterfaceName generates a name for a calico veth, given the endpoint ID
// This takes a prefix, and then truncates the EP ID.
func GenerateCaliInterfaceName(prefix, epID string) (string, error) {
	if len(prefix) > 4 {
		return "", errors.New("Prefix must be 4 characters or less.")
	}
	return fmt.Sprintf("%v%v", prefix, epID[:11]), nil
}
