package utils

import (
	"github.com/labstack/echo/log"
	"encoding/json"
)

func LogJSONMessage(logger log.Logger, formattedMessage string, data interface{}) {
	requestJSON, err := json.Marshal(data)
	if err != nil {
		logger.Error(err)
		return
	}
	logger.Debugf(formattedMessage, requestJSON)
}