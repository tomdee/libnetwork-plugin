package handlers

import (
	"net/http"

	"github.com/labstack/echo"
	"github.com/libnetwork-plugin/libnetwork-go/context"
	"github.com/libnetwork-plugin/libnetwork-go/errors"
	"github.com/libnetwork-plugin/libnetwork-go/handlers/responses"
	"github.com/tigera/libcalico-go/lib/api"
	"github.com/tigera/libcalico-go/lib/net"
)

const (
	poolIDV4 = "CalicoPoolIPv4"
	poolIDV6 = "CalicoPoolIPv6"

	poolCIDRV4 = "0.0.0.0/0"
	poolCIDRV6 = "::/0"

	gatewayCIDRV4 = "0.0.0.0/0"
	gatewayCIDRV6 = "::/0"
)

func PluginActivateHandler(c echo.Context) error {
	return c.JSON(http.StatusAccepted, responses.PluginActivate)
}

func IPAMDriverGetDefaultAddressSpaces(c echo.Context) error {
	return c.JSON(http.StatusAccepted, responses.GetDefaultAddressSpaces)
}

func IPAMDriverRequestPool(c echo.Context) error {
	clientContext := c.(*context.ClientContext)
	request := &struct {
		Pool string
		SubPool string
		V6 bool
	}{}
	clientContext.Bind(request)

	if request.SubPool != "" {
		return errors.Error(
			c,
			`Calico IPAM does not support sub pool configuration
                         on 'docker create network'.  Calico IP Pools
                         should be configured first and IP assignment is
                         from those pre-configured pools.`,
		)
	}

	client := clientContext.Client()

	if request.Pool != "" {
		poolsClient := client.Pools()
		_, ipNet, err := net.ParseCIDR(request.Pool)
		if err != nil {
			return errors.Error(c, "Invalid CIDR")
		}
		pools, err := poolsClient.List(api.PoolMetadata{CIDR: *ipNet})
		if err != nil || len(pools.Items) < 1 {
			return errors.Error(
				c,
				`The requested subnet must match the CIDR of a
				 configured Calico IP Pool.`,
			)
		}
	}

	type response struct {
		PoolID, Pool string
		Data map[string]string
	}

	var resp *response

	if request.V6 {
		resp = &response{
			PoolID: poolIDV6,
			Pool: poolCIDRV6 ,
			Data: map[string]string{"com.docker.network.gateway": gatewayCIDRV6},
		}
	} else {
		resp = &response{
			PoolID: poolIDV4,
			Pool: poolCIDRV4,
			Data: map[string]string{"com.docker.network.gateway": gatewayCIDRV4},
		}
	}

	if request.Pool != "" {
		resp.Pool = request.Pool
	}


	return c.JSON(http.StatusAccepted, resp)
}
