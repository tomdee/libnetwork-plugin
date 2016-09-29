package main

import (
	"log"

	"os"

	"github.com/docker/go-plugins-helpers/network"
	"github.com/libnetwork-plugin/libnetwork-go/driver"
	"github.com/tigera/libcalico-go/lib/api"

	libnetworkDatastore "github.com/libnetwork-plugin/libnetwork-go/datastore"
	datastoreClient "github.com/tigera/libcalico-go/lib/client"
)

var (
	config    *api.ClientConfig
	client    *datastoreClient.Client
	datastore libnetworkDatastore.Datastore

	logger *log.Logger
)

func init() {
	var err error

	if config, err = datastoreClient.LoadClientConfig(""); err != nil {
		panic(err)
	}
	if client, err = datastoreClient.New(*config); err != nil {
		panic(err)
	}
	if datastore, err = libnetworkDatastore.New(*config); err != nil {
		panic(err)
	}

	logger = log.New(os.Stdout, "", log.LstdFlags)
}

func main() {
	h := network.NewHandler(driver.NewNetworkDriver(client, datastore, logger))
	err := h.ServeUnix("root", "calico")
	log.Fatal(err)
}
