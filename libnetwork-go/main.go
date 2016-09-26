package main

import (
	"log"

	"github.com/docker/go-plugins-helpers/network"
	"github.com/libnetwork-plugin/libnetwork-go/driver"
)


func main() {
	h := network.NewHandler(driver.CalicoDriver{})
	err := h.ServeUnix("root", "calico")
	log.Fatal(err)
}
