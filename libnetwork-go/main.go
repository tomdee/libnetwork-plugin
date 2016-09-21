package main

import (
    	"github.com/labstack/echo"
    	"github.com/labstack/echo/engine/standard"
	"fmt"
	"github.com/libnetwork-plugin/libnetwork-go/context"
	"github.com/libnetwork-plugin/libnetwork-go/handlers"
	"os"
)

var serverPort string

func init() {
	serverPort = os.Getenv("PLUGIN_SERVER_PORT")
}


func main() {
    	e := echo.New()

	e.Use(func(h echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			cc := &context.ClientContext{c}
			return h(cc)
		}
	})

	e.POST("/Plugin.Activate", handlers.PluginActivateHandler)
	e.POST("/IpamDriver.GetDefaultAddressSpaces", handlers.IPAMDriverGetDefaultAddressSpaces)

    	e.Run(standard.New(fmt.Sprintf(":%v", serverPort)))
}