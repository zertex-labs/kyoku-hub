package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	mw "github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/middleware"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/database/queries"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/fsagent"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/model"
)

type AgentHandler struct {
	queries      *queries.Queries
	agentManager *fsagent.Manager
}

func NewAgentHandler(q *queries.Queries, am *fsagent.Manager) *AgentHandler {
	return &AgentHandler{queries: q, agentManager: am}
}

func (h *AgentHandler) Register(g *echo.Group) {
	// Token generation + agent listing/deletion require session auth
	g.POST("/token", h.CreateToken, mw.AuthRequired(h.queries))
	g.GET("/list", h.ListAgents, mw.AuthRequired(h.queries))
	g.DELETE("/:id", h.DeleteAgent, mw.AuthRequired(h.queries))

	// Registration uses the token itself for auth (no session needed)
	g.POST("/register", h.RegisterAgent)
}

// CreateToken generates a short-lived agent registration token for the authenticated user.
func (h *AgentHandler) CreateToken(c echo.Context) error {
	user := c.Get("user").(*model.User)

	token, err := h.queries.CreateAgentToken(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create token"})
	}

	return c.JSON(http.StatusOK, map[string]string{"token": token})
}

// RegisterAgent registers a new fs agent using a one-time token.
func (h *AgentHandler) RegisterAgent(c echo.Context) error {
	var body struct {
		Token     string `json:"token"`
		PublicKey string `json:"public_key"`
		Label     string `json:"label"`
	}
	if err := c.Bind(&body); err != nil || body.Token == "" || body.PublicKey == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing token or public_key"})
	}

	ctx := c.Request().Context()

	// Consume token (validates + deletes in one atomic operation)
	userID, err := h.queries.ConsumeAgentToken(ctx, body.Token)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid or expired token"})
	}

	agent, err := h.queries.CreateFsAgent(ctx, userID, body.Label, body.PublicKey)
	if err != nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"agent_id": agent.ID,
		"label":    agent.Label,
	})
}

// ListAgents returns all fs agents for the authenticated user, including
// live connection status.
func (h *AgentHandler) ListAgents(c echo.Context) error {
	user := c.Get("user").(*model.User)

	agents, err := h.queries.ListFsAgents(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list agents"})
	}

	for i := range agents {
		agents[i].Connected = h.agentManager.IsConnected(user.ID, agents[i].ID)
	}

	return c.JSON(http.StatusOK, agents)
}

// DeleteAgent removes an fs agent registration and tears down its live
// connection and cached scan state.
func (h *AgentHandler) DeleteAgent(c echo.Context) error {
	user := c.Get("user").(*model.User)
	agentID := c.Param("id")

	if err := h.queries.DeleteFsAgent(c.Request().Context(), agentID, user.ID); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "agent not found"})
	}

	h.agentManager.DropAgent(user.ID, agentID)

	return c.JSON(http.StatusOK, map[string]string{"ok": "true"})
}
