package logger

import (
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New returns a logger that prefixes every line with the service name.
//
// Output format (dev):  timestamp    <service>    LEVEL    caller    msg    fields
// Output format (prod): JSON with "service" field
//
// LOG_LEVEL env controls the minimum level (debug/info/warn/error).
// Defaults to debug in dev, info in production.
func New(service string) *zap.Logger {
	isProduction := os.Getenv("ENV") == "production"

	defaultLevel := zapcore.DebugLevel
	if isProduction {
		defaultLevel = zapcore.InfoLevel
	}
	level := parseLevel(os.Getenv("LOG_LEVEL"), defaultLevel)

	if isProduction {
		cfg := zap.NewProductionConfig()
		cfg.Level = zap.NewAtomicLevelAt(level)
		log, _ := cfg.Build(zap.Fields(zap.String("service", service)))
		return log
	}

	encCfg := zap.NewDevelopmentEncoderConfig()
	encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encCfg.EncodeLevel = func(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(service)
		zapcore.CapitalColorLevelEncoder(l, enc)
	}
	encCfg.ConsoleSeparator = "\t"

	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encCfg),
		zapcore.AddSync(os.Stderr),
		level,
	)

	return zap.New(core, zap.AddCaller(), zap.WithCaller(true))
}

func parseLevel(s string, fallback zapcore.Level) zapcore.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return fallback
	}
}
