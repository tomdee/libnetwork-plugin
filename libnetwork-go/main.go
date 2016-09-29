package main

import (
	"log"

	"os"

	"github.com/docker/go-plugins-helpers/network"
	"github.com/libnetwork-plugin/libnetwork-go/driver"
	datastoreClient "github.com/tigera/libcalico-go/lib/client"
)

var (
	client datastoreClient.Client
	config api.ClientConfig
	logger *log.Logger
)

func init() {
	var err error

	if config, err = datastoreClient.LoadClientConfig(""); err != nil {
		panic(err)
	}
	if client, err = datastoreClient.New(config); err != nil {
		panic(err)
	}

	logger = log.New(os.Stdout, "", log.LstdFlags)
}

func main() {
	h := network.NewHandler(driver.NewNetworkDriver(client, logger))
	err := h.ServeUnix("root", "calico")
	log.Fatal(err)
}
