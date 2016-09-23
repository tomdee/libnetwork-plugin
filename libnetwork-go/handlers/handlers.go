package handlers

import (
	"net/http"

	"github.com/labstack/echo"
	"github.com/libnetwork-plugin/libnetwork-go/context"
	"github.com/libnetwork-plugin/libnetwork-go/errors"
	"github.com/libnetwork-plugin/libnetwork-go/handlers/responses"
	"github.com/tigera/libcalico-go/lib/api"
	caliconet "github.com/tigera/libcalico-go/lib/net"
	"github.com/libnetwork-plugin/libnetwork-go/utils"
	datastoreClient "github.com/tigera/libcalico-go/lib/client"
	"net"
	"fmt"
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
	poolCIDRV4 = "0.0.0.0/0"
	poolCIDRV6 = "::/0"
	gatewayCIDRV4 = "0.0.0.0/0"
	gatewayCIDRV6 = "::/0"
)

func PluginActivateHandler(c echo.Context) error {
	clientContext := c.(*context.ClientContext)
	utils.LogJSONMessage(clientContext.Logger(), "Activate response JSON=%v", responses.PluginActivate)

	return c.JSON(http.StatusAccepted, responses.PluginActivate)
}

func IPAMDriverGetDefaultAddressSpaces(c echo.Context) error {
	clientContext := c.(*context.ClientContext)
	utils.LogJSONMessage(
		clientContext.Logger(),
		"GetDefaultAddressSpace response JSON=%v",
		responses.GetDefaultAddressSpaces,
	)

	return c.JSON(http.StatusAccepted, responses.GetDefaultAddressSpaces)
}

func IPAMDriverRequestPool(c echo.Context) error {
	clientContext := c.(*context.ClientContext)
	client := clientContext.Client()
	logger := clientContext.Logger()

	request := &struct {
		Pool string
		SubPool string
		V6 bool
	}{}
	clientContext.Bind(request)
	utils.LogJSONMessage(logger, "RequestPool JSON=%s", request)

	// Calico IPAM does not allow you to request SubPool.
	if request.SubPool != "" {
		err := errors.Error(
			c,
			"Calico IPAM does not support sub pool configuration " +
                        "on 'docker create network'. Calico IP Pools " +
                        "should be configured first and IP assignment is " +
                        "from those pre-configured pools.",
		)
		logger.Error(err)
		return err
	}

	// If a pool (subnet on the CLI) is specified, it must match one of the
    	// preconfigured Calico pools.
	if request.Pool != "" {
		poolsClient := client.Pools()
		_, ipNet, err := caliconet.ParseCIDR(request.Pool)
		if err != nil {
			err := errors.Error(c, "Invalid CIDR")
			logger.Error(err)
			return err
		}
		pools, err := poolsClient.List(api.PoolMetadata{CIDR: *ipNet})
		if err != nil || len(pools.Items) < 1 {
			err := errors.Error(
				c,
				"The requested subnet must match the CIDR of a " +
				"configured Calico IP Pool.",
			)
			logger.Error(err)
			return err
		}
	}

	var resp *responses.IPAMDriverRequestPoolResponse

	// If a subnet has been specified we use that as the pool ID. Otherwise, we
    	// use static pool ID and CIDR to indicate that we are assigning from all of
    	// the pools.
	// The meta data includes a dummy gateway address.  This prevents libnetwork
	// from requesting a gateway address from the pool since for a Calico
	// network our gateway is set to our host IP.
	if request.V6 {
		resp = &responses.IPAMDriverRequestPoolResponse{
			PoolID: poolIDV6,
			Pool: poolCIDRV6 ,
			Data: map[string]string{"com.docker.network.gateway": gatewayCIDRV6},
		}
	} else {
		resp = &responses.IPAMDriverRequestPoolResponse{
			PoolID: poolIDV4,
			Pool: poolCIDRV4,
			Data: map[string]string{"com.docker.network.gateway": gatewayCIDRV4},
		}
	}

	utils.LogJSONMessage(logger, "RequestPool response JSON=%v", resp)

	return c.JSON(http.StatusAccepted, resp)
}

func IPAMDriverReleasePool(c echo.Context) error {
	clientContext := c.(*context.ClientContext)
	logger := clientContext.Logger()

	request := &struct {
		PoolID string
	}{}
	clientContext.Bind(request)
	utils.LogJSONMessage(logger, "ReleasePool JSON=%s", request)

	resp := map[string]string{}

	utils.LogJSONMessage(logger, "ReleasePool response JSON=%s", resp)

	return c.JSON(http.StatusAccepted, resp)
}

func IPAMDriverRequestAddress(c echo.Context) error {
	hostname, err := utils.GetHostname()
	if err != nil {
		return errors.Error(c, err.Error())
	}

	clientContext := c.(*context.ClientContext)
	client := clientContext.Client()
	logger := clientContext.Logger()

	request := &struct {
		PoolID string
		Address string
	}{}
	clientContext.Bind(request)
	utils.LogJSONMessage(logger, "RequestAddress JSON=%s", request)

	var (
		version int
		pool *api.Pool
		IPs []caliconet.IP
	)

	if request.Address == "" {
		var (
			numV4 int
			numV6 int
			poolV4 *caliconet.IPNet
			poolV6 *caliconet.IPNet
		)
		logger.Debug("Auto assigning IP from Calico pools")

		// No address requested, so auto assign from our pools.  If the pool ID
		// is one of the fixed IDs then assign from across all configured pools,
		// otherwise assign from the requested pool
		if request.PoolID == poolIDV4 {
			version = 4
		} else if request.PoolID == poolIDV6 {
			version = 6
		} else {
			poolsClient := client.Pools()
			_, ipNet, err := caliconet.ParseCIDR(request.PoolID)

			if err != nil {
				return errors.Error(c, err.Error())
			}
			pool, err = poolsClient.Get(api.PoolMetadata{CIDR: *ipNet})
			if err != nil {
				message := "The network references a Calico pool which " +
					"has been deleted. Please re-instate the " +
                                	"Calico pool before using the network."
				logger.Error(err)
				return errors.Error(c, message)
			}
			version = ipNet.Version()
		}

		if version == 4 {
			numV4 = 1
			numV6 = 0
			poolV4 = &caliconet.IPNet{pool.Metadata.CIDR.IPNet}
		} else {
			numV4 = 0
			numV6 = 1
			poolV6 = &caliconet.IPNet{pool.Metadata.CIDR.IPNet}
		}

		// Auto assign an IP based on whether the IPv4 or IPv6 pool was selected.
		// We auto-assign from all available pools with affinity based on our
		// host.
		IPsV4, IPsV6, err := client.IPAM().AutoAssign(
			datastoreClient.AutoAssignArgs{
				Num4: numV4,
				Num6: numV6,
				Hostname: hostname,
				IPv4Pool: poolV4,
				IPv6Pool: poolV6,
			},
		)
		IPs = append(IPsV4, IPsV6...)
		if err != nil || len(IPs) == 0 {
			message := "There are no available IP addresses in the configured Calico IP pools"
			logger.Error(message)
			return errors.Error(c, message)
		}

	} else {
		logger.Debug("Reserving a specific address in Calico pools")
		ip := net.ParseIP(request.Address)
		err := client.IPAM().AssignIP(
			datastoreClient.AssignIPArgs{
				IP: caliconet.IP{ip},
				Hostname: hostname,
			},
		)
		if err != nil {
			logger.Error(err)
			return errors.Error(c, err.Error())
		}
		IPs = []caliconet.IP{{ip}}
	}

	// We should only have one IP address assigned at this point.
	if len(IPs) != 1 {
		message := "Unexpected number of assigned IP addresses"
		logger.Error(message)
		return errors.Error(c, message)
	}
	
	_, ipNet, err := caliconet.ParseCIDR(fmt.Sprint(IPs[0]))

	// Return the IP as a CIDR.
	resp := responses.IPAMDriverRequestAddressResponse{
		Address: fmt.Sprint(ipNet),
	}

	utils.LogJSONMessage(logger, "RequestAddress response JSON=%s", resp)

	return c.JSON(http.StatusAccepted, resp)
}
