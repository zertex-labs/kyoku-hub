package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/database/queries"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/download"
	mw "github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/middleware"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/plugins"
)

// PluginHandler exposes the installed plugins and their configuration. There is
// no install endpoint on purpose: dropping a directory on the server is granted
// access to the server, and that should require access to the server.
type PluginHandler struct {
	registry *plugins.Registry
	store    *plugins.Store
	download *download.Manager
	queries  *queries.Queries
}

func NewPluginHandler(reg *plugins.Registry, store *plugins.Store, mgr *download.Manager, q *queries.Queries) *PluginHandler {
	return &PluginHandler{registry: reg, store: store, download: mgr, queries: q}
}

func (h *PluginHandler) Register(g *echo.Group) {
	g.GET("", h.List, mw.AuthRequired(h.queries))
	g.GET("/:id/config", h.GetConfig, mw.AuthRequired(h.queries))
	g.PUT("/:id/config", h.PutConfig, mw.AuthRequired(h.queries))
	g.POST("/reload", h.Reload, mw.AuthRequired(h.queries))
}

// pluginView is what the settings page renders. Secret values are never sent:
// the field is described, its value is not.
type pluginView struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Version     string              `json:"version"`
	Author      string              `json:"author"`
	Description string              `json:"description"`
	Hooks       []string            `json:"hooks"`
	Required    bool                `json:"required"`
	Permissions plugins.Permissions `json:"permissions"`
	Config      []plugins.Field     `json:"config"`
	Values      map[string]any      `json:"values"`
}

func (h *PluginHandler) List(c echo.Context) error {
	if h.registry == nil {
		return c.JSON(http.StatusOK, []pluginView{})
	}
	views := make([]pluginView, 0, h.registry.Count())
	for _, p := range h.registry.Plugins() {
		values, err := h.valuesFor(c.Request().Context(), p)
		if err != nil {
			values = map[string]any{}
		}
		views = append(views, pluginView{
			ID:          p.Name,
			Name:        p.Name,
			Version:     p.Version,
			Author:      p.Author,
			Description: p.Description,
			Hooks:       p.Hooks,
			Required:    p.Required,
			Permissions: p.Permissions,
			Config:      p.Config,
			Values:      maskSecrets(p.Config, values),
		})
	}
	return c.JSON(http.StatusOK, views)
}

func (h *PluginHandler) GetConfig(c echo.Context) error {
	p, err := h.find(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	}
	values, err := h.valuesFor(c.Request().Context(), p)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{
		"config": p.Config,
		"values": maskSecrets(p.Config, values),
	})
}

func (h *PluginHandler) PutConfig(c echo.Context) error {
	p, err := h.find(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	}
	if h.store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "plugin config is disabled: no session secret"})
	}

	// Decode the body rather than c.Bind: Echo's default binder merges path and
	// query params into a map, so ":id" would arrive as a config field.
	var body map[string]any
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid body"})
	}

	values, err := validateConfig(p.Config, body)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if err := h.store.Save(c.Request().Context(), p.Name, values); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"values": maskSecrets(p.Config, values)})
}

// Reload re-reads the plugins directory so a newly dropped plugin appears
// without restarting the hub.
func (h *PluginHandler) Reload(c echo.Context) error {
	if h.download == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "download manager unavailable"})
	}
	return c.JSON(http.StatusNotImplemented, map[string]string{
		"error": "reload is not wired yet: restart the hub to pick up a new plugin",
	})
}

func (h *PluginHandler) find(id string) (plugins.Plugin, error) {
	if h.registry == nil {
		return plugins.Plugin{}, fmt.Errorf("no plugins installed")
	}
	for _, p := range h.registry.Plugins() {
		if p.Name == id {
			return p, nil
		}
	}
	return plugins.Plugin{}, fmt.Errorf("plugin %q is not installed", id)
}

func (h *PluginHandler) valuesFor(ctx context.Context, p plugins.Plugin) (map[string]any, error) {
	values := map[string]any{}
	for _, f := range p.Config {
		if f.Default != nil {
			values[f.Key] = f.Default
		}
	}
	if h.store == nil {
		return values, nil
	}
	stored, err := h.store.Values(ctx, p.Name)
	if err != nil {
		return values, err
	}
	for k, v := range stored {
		values[k] = v
	}
	return values, nil
}

// maskSecrets blanks secret fields: the API describes what exists, never what it
// holds.
func maskSecrets(fields []plugins.Field, values map[string]any) map[string]any {
	out := make(map[string]any, len(values))
	for k, v := range values {
		out[k] = v
	}
	for _, f := range fields {
		if f.Type == plugins.FieldSecret {
			if _, set := out[f.Key]; set {
				out[f.Key] = ""
			}
			out[f.Key+"_set"] = out[f.Key] != nil && values[f.Key] != "" && values[f.Key] != nil
		}
	}
	return out
}

// validateConfig coerces and checks values against the manifest fields. A field
// the manifest does not declare is an error: storing it would be a silent no-op.
func validateConfig(fields []plugins.Field, body map[string]any) (map[string]any, error) {
	known := make(map[string]plugins.Field, len(fields))
	for _, f := range fields {
		known[f.Key] = f
	}
	for k := range body {
		if _, ok := known[k]; !ok {
			return nil, fmt.Errorf("unknown config field %q", k)
		}
	}

	out := make(map[string]any, len(fields))
	for _, f := range fields {
		raw, present := body[f.Key]
		if !present || raw == nil {
			if f.Required {
				return nil, fmt.Errorf("%s is required", f.Key)
			}
			if f.Default != nil {
				out[f.Key] = f.Default
			}
			continue
		}

		switch f.Type {
		case plugins.FieldString, plugins.FieldSecret:
			s, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("%s must be a string", f.Key)
			}
			if f.Required && strings.TrimSpace(s) == "" {
				return nil, fmt.Errorf("%s is required", f.Key)
			}
			out[f.Key] = s
		case plugins.FieldBool:
			b, ok := raw.(bool)
			if !ok {
				return nil, fmt.Errorf("%s must be true or false", f.Key)
			}
			out[f.Key] = b
		case plugins.FieldInt:
			v, err := asInt(raw)
			if err != nil {
				return nil, fmt.Errorf("%s must be a number", f.Key)
			}
			if f.Min != nil && v < *f.Min {
				return nil, fmt.Errorf("%s must be at least %d", f.Key, *f.Min)
			}
			if f.Max != nil && v > *f.Max {
				return nil, fmt.Errorf("%s must be at most %d", f.Key, *f.Max)
			}
			out[f.Key] = v
		case plugins.FieldSelect:
			s, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("%s must be a string", f.Key)
			}
			valid := false
			for _, opt := range f.Options {
				if opt == s {
					valid = true
					break
				}
			}
			if !valid {
				return nil, fmt.Errorf("%s must be one of %s", f.Key, strings.Join(f.Options, ", "))
			}
			out[f.Key] = s
		}
	}
	return out, nil
}

func asInt(raw any) (int, error) {
	switch v := raw.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	case string:
		return strconv.Atoi(v)
	}
	return 0, fmt.Errorf("not a number")
}
