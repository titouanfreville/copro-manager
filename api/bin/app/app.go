package main

import (
	"context"
	"os"
	"strings"
	"time"

	fs "cloud.google.com/go/firestore"
	"go.uber.org/fx"
	uberzap "go.uber.org/zap"

	authadapter "github.com/titouanfreville/copro-manager/api/src/adapters/auth"
	categoriesadapter "github.com/titouanfreville/copro-manager/api/src/adapters/categories"
	coprosadapter "github.com/titouanfreville/copro-manager/api/src/adapters/copros"
	expensesadapter "github.com/titouanfreville/copro-manager/api/src/adapters/expenses"
	foyersadapter "github.com/titouanfreville/copro-manager/api/src/adapters/foyers"
	usersadapter "github.com/titouanfreville/copro-manager/api/src/adapters/users"
	"github.com/titouanfreville/copro-manager/api/src/core/config"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/categories"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/expenses"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/foyers"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/home"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/users"
	"github.com/titouanfreville/copro-manager/api/src/servers/api"
	"github.com/titouanfreville/copro-manager/api/src/servers/api/middlewares"
	"github.com/titouanfreville/copro-manager/api/src/services/firebase"
	"github.com/titouanfreville/copro-manager/api/src/services/firestore"
	"github.com/titouanfreville/copro-manager/api/src/services/fxapp"
	"github.com/titouanfreville/copro-manager/api/src/services/otel"
	"github.com/titouanfreville/copro-manager/api/src/services/storage"
	"github.com/titouanfreville/copro-manager/api/src/services/zap"
)

const timeout = 30 * time.Second

func main() {
	app := fx.New(
		fx.Provide(
			func() *config.Config {
				var confFiles []string

				confFile := os.Getenv("CONFIG_FILE")
				if confFile == "" {
					confFiles = []string{"conf/main.yml"}
				} else {
					confFiles = strings.Split(confFile, string(os.PathListSeparator))
				}

				return config.NewConfigFromYAML(confFiles...)
			},

			func(cfg *config.Config) *uberzap.Logger {
				log, _ := zap.NewZap(cfg.Logger)

				return log
			},

			func(cfg *config.Config) (*fs.Client, error) {
				return firestore.NewClient(cfg.Firestore)
			},

			func(cfg *config.Config) (*storage.Client, error) {
				return storage.NewClient(cfg.Storage)
			},

			firebase.NewApp,
			firebase.NewAuthClient,
			firebase.NewAdminClient,

			fx.Annotate(foyersadapter.NewStore, fx.As(new(interfaces.FoyersStore))),
			fx.Annotate(coprosadapter.NewStore, fx.As(new(interfaces.CoprosStore))),
			fx.Annotate(usersadapter.NewStore, fx.As(new(interfaces.UsersStore))),
			fx.Annotate(categoriesadapter.NewStore, fx.As(new(interfaces.CategoriesStore))),
			fx.Annotate(expensesadapter.NewStore, fx.As(new(interfaces.ExpensesStore))),
			fx.Annotate(authadapter.NewFirebaseProvisioner, fx.As(new(interfaces.AuthProvisioner))),

			home.New,
			users.New,
			func(u users.Usecases) interfaces.UsersService { return u },
			foyers.New,
			categories.New,
			expenses.New,
			usecases.New,

			func(cfg *config.Config, logger *uberzap.Logger, auth firebase.AuthClient) *middlewares.Middlewares {
				return middlewares.NewMiddlewares(cfg.Middlewares, logger, auth)
			},
		),

		api.Transport,

		fx.Invoke(
			func(cfg *config.Config, logger *uberzap.Logger) {
				otel.Init(cfg.OTEL, logger)
			},
		),
		fx.Invoke(
			func(store interfaces.CategoriesStore, logger *uberzap.Logger) error {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := categories.EnsureSeeded(ctx, store); err != nil {
					logger.Named("bootstrap").Error("category seed failed", uberzap.Error(err))
					return err
				}
				return nil
			},
		),
		fx.Invoke(api.Run),
	)

	fxapp.Start(app, timeout)
	<-app.Done()
	fxapp.Shutdown(app, timeout)
}
