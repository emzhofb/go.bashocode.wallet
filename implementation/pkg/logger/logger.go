package logger

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type correlationKey struct{}

var CorrelationKey correlationKey

func NewLogger(env string, serviceName string) *zap.Logger {
	var config zap.Config
	if env == "production" {
		config = zap.NewProductionConfig()
	} else {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	config.EncoderConfig.TimeKey = "ts"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	log, _ := config.Build(zap.AddCallerSkip(1))
	return log.With(zap.String("service_name", serviceName))
}

func ContextWithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, CorrelationKey, correlationID)
}

func GetCorrelationID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(CorrelationKey).(string); ok {
		return v
	}
	return ""
}

func WithCorrelationID(ctx context.Context, log *zap.Logger) *zap.Logger {
	corID := GetCorrelationID(ctx)
	if corID != "" {
		return log.With(zap.String("correlation_id", corID))
	}
	return log
}
