package logger

import "go.uber.org/zap"

// New creates a new zap logger for pro or dev depending of the env
func New(env string) *zap.Logger {
	if env == "production" {
		return zap.Must(zap.NewProduction())
	}

	return zap.Must(zap.NewDevelopment())
}
