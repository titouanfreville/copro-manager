package zap

import (
	"go.uber.org/zap"
)

const (
	DevEnv  = "dev"
	ProdEnv = "prod"
)

// Config allows logger configuration.
type Config struct {
	Type string `yaml:"type"`
	Name string `yaml:"name"`
	Env  string `yaml:"env"`
}

// NewZap returns a new ZAP logger instance configured for the given environment.
func NewZap(conf Config) (logger *zap.Logger, err error) {
	switch conf.Env {
	case ProdEnv:
		logger, err = zap.NewProduction()
	case DevEnv:
		logger, err = zap.NewDevelopment()
	default:
		logger = zap.NewExample()
	}

	if err == nil {
		logger = logger.Named(conf.Type).Named(conf.Name)
	}

	return
}
