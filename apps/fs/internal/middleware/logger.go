package middleware

import (
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

func ZapLogger(log *zap.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()

			err := next(c)
			if err != nil {
				c.Error(err)
			}

			req := c.Request()
			res := c.Response()
			latency := time.Since(start)

			fields := []zap.Field{
				zap.String("method", req.Method),
				zap.String("uri", req.RequestURI),
				zap.Int("status", res.Status),
				zap.Duration("latency", latency),
				zap.String("remote_ip", c.RealIP()),
			}

			if err != nil {
				fields = append(fields, zap.Error(err))
			}

			switch {
			case res.Status >= 500:
				log.Error("request", fields...)
			case res.Status >= 400:
				log.Warn("request", fields...)
			default:
				log.Info("request", fields...)
			}

			return nil
		}
	}
}

