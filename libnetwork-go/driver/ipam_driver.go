package driver

import (
	"fmt"
	"log"
	"net"

	"errors"

	"github.com/docker/go-plugins-helpers/ipam"
	logutils "github.com/libnetwork-plugin/libnetwork-go/utils/log"
	osutils "github.com/libnetwork-plugin/libnetwork-go/utils/os"
	"github.com/tigera/libcalico-go/lib/api"
	datastoreClient "github.com/tigera/libcalico-go/lib/client"
	caliconet "github.com/tigera/libcalico-go/lib/net"
)

const (
	// Calico IPAM module does not allow selection of pools from which to allocate
	// IP addresses.  The pool ID, which has to be supplied in the libnetwork IPAM
	// API is therefore fixed.  We use different values for IPv4 and IPv6 so that
	// during allocation we know which IP version to use.
	poolIDV4 = "CalicoPoolIPv4"
	poolIDV6 = "CalicoPoolIPv6"

	// Fix pool and gateway CIDRs.  As per comment above, Calico IPAM does not allow
	// assignment from a specific pool, so we choose a dummy value that will not be
	// used in practise.  A 0/0 value is used for both IPv4 and IPv6.  This value is
	// also used by the Network Driver to indicate that the Calico IPAM driver was
	// used rather than the default libnetwork IPAM driver - this is useful because
	// Calico Network Driver behavior depends on whether our IPAM driver was used or
	// not.
	poolCIDRV4    = "0.0.0.0/0"
	poolCIDRV6    = "::/0"
	gatewayCIDRV4 = "0.0.0.0/0"
	gatewayCIDRV6 = "::/0"
)

type IpamDriver struct {
	client *datastoreClient.Client
	logger *log.Logger
}

func (i IpamDriver) GetDefaultAddressSpaces() (*ipam.AddressSpacesResponse, error) {
	resp := &ipam.AddressSpacesResponse{
		LocalDefaultAddressSpace:  "CalicoLocalAddressSpace",
		GlobalDefaultAddressSpace: "CalicoGlobalAddressSpace",
	}
	logutils.JSONMessage(i.logger, "GetDefaultAddressSpace response JSON=%v", resp)
	return resp, nil
}

func (i IpamDriver) RequestPool(request *ipam.RequestPoolRequest) (*ipam.RequestPoolResponse, error) {
	logutils.JSONMessage(i.logger, "RequestPool JSON=%s", request)

	// Calico IPAM does not allow you to request SubPool.
	if request.SubPool != "" {
		err := errors.New(
			"Calico IPAM does not support sub pool configuration " +
				"on 'docker create network'. Calico IP Pools " +
				"should be configured first and IP assignment is " +
				"from those pre-configured pools.",
		)
		i.logger.Println(err)
		return nil, err
	}

	// If a pool (subnet on the CLI) is specified, it must match one of the
	// preconfigured Calico pools.
	if request.Pool != "" {
		poolsClient := i.client.Pools()
		_, ipNet, err := caliconet.ParseCIDR(request.Pool)
		if err != nil {
			err := errors.New("Invalid CIDR")
			i.logger.Println(err)
			return nil, err
		}
		pools, err := poolsClient.List(api.PoolMetadata{CIDR: *ipNet})
		if err != nil || len(pools.Items) < 1 {
			err := errors.New(
				"The requested subnet must match the CIDR of a " +
					"configured Calico IP Pool.",
			)
			i.logger.Println(err)
			return nil, err
		}
	}

	var resp *ipam.RequestPoolResponse

	// If a subnet has been specified we use that as the pool ID. Otherwise, we
	// use static pool ID and CIDR to indicate that we are assigning from all of
	// the pools.
	// The meta data includes a dummy gateway address.  This prevents libnetwork
	// from requesting a gateway address from the pool since for a Calico
	// network our gateway is set to our host IP.
	if request.V6 {
		resp = &ipam.RequestPoolResponse{
			PoolID: poolIDV6,
			Pool:   poolCIDRV6,
			Data:   map[string]string{"com.docker.network.gateway": gatewayCIDRV6},
		}
	} else {
		resp = &ipam.RequestPoolResponse{
			PoolID: poolIDV4,
			Pool:   poolCIDRV4,
			Data:   map[string]string{"com.docker.network.gateway": gatewayCIDRV4},
		}
	}

	logutils.JSONMessage(i.logger, "RequestPool response JSON=%v", resp)

	return resp, nil
}

func (i IpamDriver) ReleasePool(request *ipam.ReleasePoolRequest) error {
	logutils.JSONMessage(i.logger, "ReleasePool JSON=%s", request)

	resp := map[string]string{}
	logutils.JSONMessage(i.logger, "ReleasePool response JSON=%s", resp)
	return nil
}

func (i IpamDriver) RequestAddress(request *ipam.RequestAddressRequest) (*ipam.RequestAddressResponse, error) {
	logutils.JSONMessage(i.logger, "RequestAddress JSON=%s", request)

	hostname, err := osutils.GetHostname()
	if err != nil {
		return nil, err
	}

	var (
		version int
		pool    *api.Pool
		IPs     []caliconet.IP
	)

	if request.Address == "" {
		var (
			numV4  int
			numV6  int
			poolV4 *caliconet.IPNet
			poolV6 *caliconet.IPNet
		)
		i.logger.Println("Auto assigning IP from Calico pools")

		// No address requested, so auto assign from our pools.  If the pool ID
		// is one of the fixed IDs then assign from across all configured pools,
		// otherwise assign from the requested pool
		if request.PoolID == poolIDV4 {
			version = 4
		} else if request.PoolID == poolIDV6 {
			version = 6
		} else {
			poolsClient := i.client.Pools()
			_, ipNet, err := caliconet.ParseCIDR(request.PoolID)

			if err != nil {
				return nil, err
			}
			pool, err = poolsClient.Get(api.PoolMetadata{CIDR: *ipNet})
			if err != nil {
				message := "The network references a Calico pool which " +
					"has been deleted. Please re-instate the " +
					"Calico pool before using the network."
				i.logger.Println(err)
				return nil, errors.New(message)
			}
			version = ipNet.Version()
		}

		if version == 4 {
			numV4 = 1
			numV6 = 0
			poolV4 = &caliconet.IPNet{IPNet: pool.Metadata.CIDR.IPNet}
		} else {
			numV4 = 0
			numV6 = 1
			poolV6 = &caliconet.IPNet{IPNet: pool.Metadata.CIDR.IPNet}
		}

		// Auto assign an IP based on whether the IPv4 or IPv6 pool was selected.
		// We auto-assign from all available pools with affinity based on our
		// host.
		IPsV4, IPsV6, err := i.client.IPAM().AutoAssign(
			datastoreClient.AutoAssignArgs{
				Num4:     numV4,
				Num6:     numV6,
				Hostname: hostname,
				IPv4Pool: poolV4,
				IPv6Pool: poolV6,
			},
		)
		IPs = append(IPsV4, IPsV6...)
		if err != nil || len(IPs) == 0 {
			err := errors.New("There are no available IP addresses in the configured Calico IP pools")
			i.logger.Println(err)
			return nil, err
		}

	} else {
		i.logger.Println("Reserving a specific address in Calico pools")
		ip := net.ParseIP(request.Address)
		err := i.client.IPAM().AssignIP(
			datastoreClient.AssignIPArgs{
				IP:       caliconet.IP{IP: ip},
				Hostname: hostname,
			},
		)
		if err != nil {
			i.logger.Println(err)
			return nil, err
		}
		IPs = []caliconet.IP{caliconet.IP{IP: ip}}
	}

	// We should only have one IP address assigned at this point.
	if len(IPs) != 1 {
		err := errors.New("Unexpected number of assigned IP addresses")
		i.logger.Println(err)
		return nil, err
	}

	_, ipNet, err := caliconet.ParseCIDR(fmt.Sprint(IPs[0]))
	if err != nil {
		i.logger.Println(err)
		return nil, err
	}

	// Return the IP as a CIDR.
	resp := &ipam.RequestAddressResponse{
		Address: fmt.Sprint(ipNet),
	}

	logutils.JSONMessage(i.logger, "RequestAddress response JSON=%s", resp)

	return resp, nil
}

func (i IpamDriver) ReleaseAddress(request *ipam.ReleaseAddressRequest) error {
	logutils.JSONMessage(i.logger, "ReleaseAddress JSON=%s", request)

	ip := caliconet.IP{IP: net.ParseIP(request.Address)}

	// Unassign the address.  This handles the address already being unassigned
	// in which case it is a no-op.  The release_ips call may raise a
	// RuntimeError if there are repeated clashing updates to the same IP block,
	// this is not an expected condition.
	_, err := i.client.IPAM().ReleaseIPs([]caliconet.IP{ip})
	if err != nil {
		i.logger.Println(err)
		return err
	}

	resp := map[string]string{}

	logutils.JSONMessage(i.logger, "ReleaseAddress response JSON=%s", resp)

	return nil
}
