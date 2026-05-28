package middleware

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/database/queries"
)

func AuthRequired(q *queries.Queries) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			cookie, err := c.Cookie("kyoku_session")
			if err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			}
			user, err := q.GetSessionWithUser(c.Request().Context(), cookie.Value)
			if err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			}
			c.Set("user", user)
			return next(c)
		}
	}
}
