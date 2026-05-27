package handler

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/sse"
)

type EventsHandler struct {
	manager *sse.Manager
}

func NewEventsHandler(manager *sse.Manager) *EventsHandler {
	return &EventsHandler{manager: manager}
}

func (h *EventsHandler) Stream(c echo.Context) error {
	w := c.Response()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Send initial connection event
	fmt.Fprint(w, "event: connected\ndata: {}\n\n")
	w.Flush()

	client := h.manager.Register()
	defer h.manager.Unregister(client)

	ctx := c.Request().Context()
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-client.Events:
			if !ok {
				return nil
			}
			_, err := w.Write(msg)
			if err != nil {
				return nil
			}
			w.Flush()
		}
	}
}
