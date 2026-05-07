package config

import (
	"fmt"

	"github.com/titouanfreville/copro-manager/api/src/servers/cors"
)

// Config holds the HTTP API server configuration.
type Config struct {
	Scheme  string      `yaml:"scheme"`
	Host    string      `yaml:"host"`
	Port    int         `yaml:"port"`
	NoCache bool        `yaml:"no_cache"`
	CORS    cors.Config `yaml:"cors"`
}

// GetAddress returns the host:port address string.
func (c *Config) GetAddress() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
