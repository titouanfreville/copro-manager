package api

import (
	"go.uber.org/fx"
	"go.uber.org/zap"

	rootconfig "github.com/titouanfreville/copro-manager/api/src/core/config"
	"github.com/titouanfreville/copro-manager/api/src/servers"
	"github.com/titouanfreville/copro-manager/api/src/servers/api/config"
	"github.com/titouanfreville/copro-manager/api/src/services/fxapp"
)

// Transport contains all the dependencies used to run the API transport layer.
var Transport = fx.Provide(
	func(conf *rootconfig.Config) *config.Config {
		return &conf.API
	},

	fx.Annotated{Name: "api", Target: NewHTTP},
)

// FxParams is the parameter used by uber-go/fx for the dependency injection.
type FxParams struct {
	fx.In
	Lifecycle  fx.Lifecycle
	Shutdowner fx.Shutdowner
	Logger     *zap.Logger
	Transport  servers.TCP `name:"api"`
}

// Run registers the API transport in the FX lifecycle.
func Run(p FxParams) {
	fxServer := fxapp.NewTCPServer(p.Lifecycle, p.Shutdowner, p.Logger)
	fxServer.Run("http", p.Transport)
}
