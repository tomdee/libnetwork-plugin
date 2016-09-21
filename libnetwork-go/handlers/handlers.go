package handlers

import (
	"net/http"
	"github.com/libnetwork-plugin/libnetwork-go/handlers/responses"
	"github.com/labstack/echo"
)

func PluginActivateHandler(c echo.Context) error {
	return c.JSON(http.StatusAccepted, responses.PluginActivate)
}

func IPAMDriverGetDefaultAddressSpaces(c echo.Context) error {
	return c.JSON(http.StatusAccepted, responses.GetDefaultAddressSpaces)
}
