package config

import (
	"fmt"

	uberconfig "go.uber.org/config"

	apiconfig "github.com/titouanfreville/copro-manager/api/src/servers/api/config"
	"github.com/titouanfreville/copro-manager/api/src/servers/api/middlewares"
	"github.com/titouanfreville/copro-manager/api/src/services/firestore"
	geminisvc "github.com/titouanfreville/copro-manager/api/src/services/gemini"
	"github.com/titouanfreville/copro-manager/api/src/services/otel"
	pushsvc "github.com/titouanfreville/copro-manager/api/src/services/push"
	"github.com/titouanfreville/copro-manager/api/src/services/storage"
	"github.com/titouanfreville/copro-manager/api/src/services/zap"
)

// Config is the root configuration struct aggregating all sub-configs.
type Config struct {
	API         apiconfig.Config   `yaml:"api"`
	Logger      zap.Config         `yaml:"logger"`
	Firestore   firestore.Config   `yaml:"firestore"`
	Storage     storage.Config     `yaml:"storage"`
	Push        pushsvc.Config     `yaml:"push"`
	Gemini      geminisvc.Config   `yaml:"gemini"`
	Middlewares middlewares.Config `yaml:"middlewares"`
	OTEL        otel.Config        `yaml:"otel"`
}

// NewConfigFromYAML loads configuration from one or more YAML files. Files
// are merged left-to-right; the right-most file wins on conflict.
//
// Environment-variable expansion (`${VAR}`) is intentionally NOT enabled —
// silent absence is bug-prone and the project rule (AGENTS.md) is to keep
// functional config in YAML, layering per-machine overrides via a local
// file pointed to by CONFIG_FILE.
func NewConfigFromYAML(paths ...string) *Config {
	opts := make([]uberconfig.YAMLOption, 0, len(paths))

	for _, path := range paths {
		opts = append(opts, uberconfig.File(path))
	}

	provider, err := uberconfig.NewYAML(opts...)
	if err != nil {
		panic(fmt.Sprintf("failed to load config from %v: %s", paths, err.Error()))
	}

	var cfg Config
	if err := provider.Get(uberconfig.Root).Populate(&cfg); err != nil {
		panic("failed to populate config: " + err.Error())
	}

	return &cfg
}
