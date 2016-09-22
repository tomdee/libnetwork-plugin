package errors

import (
	"github.com/labstack/echo"
	"net/http"
)

func Error(c echo.Context, message string) error {
	return c.JSON(
		http.StatusBadRequest,
		map[string]string {
			"Err": message,
		},
	)
}
