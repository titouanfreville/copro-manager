package otel

import "go.uber.org/zap"

// Config holds the OpenTelemetry configuration.
type Config struct {
	Enabled     bool   `yaml:"enabled"`
	ServiceName string `yaml:"service_name"`
}

// Init initializes OpenTelemetry providers if enabled.
func Init(conf Config, logger *zap.Logger) {
	log := logger.Named("OTEL")

	if !conf.Enabled {
		log.Info("OTEL disabled, skipping initialization")

		return
	}

	// TODO: Initialize OTEL providers (traces, metrics, logs)
	log.Info("OTEL initialization placeholder")
}
