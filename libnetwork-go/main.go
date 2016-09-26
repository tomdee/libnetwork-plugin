package main

import (
	"fmt"
	"log"
	"os"

	"github.com/docker/go-plugins-helpers/network"
	"github.com/libnetwork-plugin/libnetwork-go/driver"
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
	h := network.NewHandler(driver.CalicoDriver{})
	err := h.ServeTCP("calico-net", fmt.Sprintf(":%v", serverPort))
	log.Fatal(err)
}
