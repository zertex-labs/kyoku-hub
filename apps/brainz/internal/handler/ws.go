package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

type WSHandler struct {
	logger *zap.Logger
}

func NewWSHandler(logger *zap.Logger) *WSHandler {
	return &WSHandler{logger: logger}
}

func (h *WSHandler) RegisterRoutes(e *echo.Echo) {
	e.GET("/ws", h.HandleWS)
}

func (h *WSHandler) HandleWS(c echo.Context) error {
	// TODO: upgrade to websocket, authenticate hub instance, handle subscribe/unsubscribe
	h.logger.Info("ws connection attempt (not yet implemented)")
	return c.JSON(http.StatusNotImplemented, map[string]string{"error": "not implemented"})
}
