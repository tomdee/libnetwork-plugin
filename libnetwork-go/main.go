package main

import (
	"os"

	"github.com/labstack/echo"
	"github.com/labstack/echo/engine/standard"
	"github.com/libnetwork-plugin/libnetwork-go/context"
	"github.com/libnetwork-plugin/libnetwork-go/handlers"
)

const (
	defaultServerPort = "9000"
)

var (
	serverPort string
)

func init() {
	serverPort = os.Getenv("PLUGIN_SERVER_PORT")
	if serverPort == "" {
		serverPort = defaultServerPort
	}
}

func main() {
	e := echo.New()

	e.Logger().Printf("Running on port %v...", serverPort)

	e.Use(func(h echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			cc := &context.ClientContext{Context: c}
			return h(cc)
		}
	})

	e.POST("/Plugin.Activate", handlers.PluginActivateHandler)

	// IPAM driver endpoints.
	e.POST("/IpamDriver.GetDefaultAddressSpaces", handlers.IPAMDriverGetDefaultAddressSpaces)
	e.POST("/IpamDriver.RequestPool", handlers.IPAMDriverRequestPool)
	e.POST("/IpamDriver.ReleasePool", handlers.IPAMDriverReleasePool)
	e.POST("/IpamDriver.RequestAddress", handlers.IPAMDriverRequestAddress)
	e.POST("/IpamDriver.ReleaseAddress", handlers.IPAMDriverReleaseAddress)

	err := e.Run(standard.New(":" + serverPort))

	if err != nil {
		e.Logger().Printf("%v\n", err)
		return
	}

	e.Logger().Print("Exiting.")
}
