package context

import (
	"github.com/labstack/echo"
	datastoreClient "github.com/tigera/libcalico-go/lib/client"
	"github.com/pkg/errors"
)

type ClientContext struct {
	echo.Context
}

func (c *ClientContext) Client() *datastoreClient.Client {
	var config, err = datastoreClient.LoadClientConfig("")
	client, err := datastoreClient.New(*config)
	if err != nil {
		panic(errors.Wrap(err, "Can't start libnetwork-plugin"))
	}
	return client
}
