package main

import (
	"context"
	"os"
	"strings"
	"time"

	fs "cloud.google.com/go/firestore"
	"go.uber.org/fx"
	uberzap "go.uber.org/zap"

	alertsadapter "github.com/titouanfreville/copro-manager/api/src/adapters/alerts"
	authadapter "github.com/titouanfreville/copro-manager/api/src/adapters/auth"
	categoriesadapter "github.com/titouanfreville/copro-manager/api/src/adapters/categories"
	coprosadapter "github.com/titouanfreville/copro-manager/api/src/adapters/copros"
	documentsadapter "github.com/titouanfreville/copro-manager/api/src/adapters/documents"
	expensesadapter "github.com/titouanfreville/copro-manager/api/src/adapters/expenses"
	foyersadapter "github.com/titouanfreville/copro-manager/api/src/adapters/foyers"
	metersadapter "github.com/titouanfreville/copro-manager/api/src/adapters/meters"
	pushadapter "github.com/titouanfreville/copro-manager/api/src/adapters/push"
	settlementsadapter "github.com/titouanfreville/copro-manager/api/src/adapters/settlements"
	templatesadapter "github.com/titouanfreville/copro-manager/api/src/adapters/templates"
	usersadapter "github.com/titouanfreville/copro-manager/api/src/adapters/users"
	visionusageadapter "github.com/titouanfreville/copro-manager/api/src/adapters/visionusage"
	"github.com/titouanfreville/copro-manager/api/src/core/config"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/alerts"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/categories"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/documents"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/expenses"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/foyers"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/home"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/meters"
	pushuc "github.com/titouanfreville/copro-manager/api/src/domain/usecases/push"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/settlements"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/templates"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/users"
	"github.com/titouanfreville/copro-manager/api/src/servers/api"
	"github.com/titouanfreville/copro-manager/api/src/servers/api/middlewares"
	"github.com/titouanfreville/copro-manager/api/src/services/firebase"
	"github.com/titouanfreville/copro-manager/api/src/services/firestore"
	"github.com/titouanfreville/copro-manager/api/src/services/fxapp"
	"github.com/titouanfreville/copro-manager/api/src/services/otel"
	pushsvc "github.com/titouanfreville/copro-manager/api/src/services/push"
	"github.com/titouanfreville/copro-manager/api/src/services/storage"
	visionsvc "github.com/titouanfreville/copro-manager/api/src/services/vision"
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

			fx.Annotate(
				func(cfg *config.Config) (*storage.Client, error) {
					return storage.NewClient(cfg.Storage)
				},
				fx.As(new(interfaces.StorageService)),
			),

			fx.Annotate(
				func(cfg *config.Config) *pushsvc.Sender {
					return pushsvc.NewSender(cfg.Push)
				},
				fx.As(new(interfaces.PushSender)),
			),

			fx.Annotate(
				visionusageadapter.NewStore,
				fx.As(new(interfaces.VisionUsageStore)),
			),
			fx.Annotate(
				func(lc fx.Lifecycle, cfg *config.Config, usage interfaces.VisionUsageStore) (*visionsvc.Client, error) {
					c, err := visionsvc.NewClient(cfg.Vision, usage)
					if err != nil {
						return nil, err
					}
					// Register Close on shutdown so the gRPC client + auth
					// connections aren't leaked between Cloud Run instances.
					lc.Append(fx.Hook{
						OnStop: func(_ context.Context) error { return c.Close() },
					})
					return c, nil
				},
				fx.As(new(interfaces.OCRService)),
			),

			firebase.NewApp,
			firebase.NewAuthClient,
			firebase.NewAdminClient,

			fx.Annotate(foyersadapter.NewStore, fx.As(new(interfaces.FoyersStore))),
			fx.Annotate(coprosadapter.NewStore, fx.As(new(interfaces.CoprosStore))),
			fx.Annotate(usersadapter.NewStore, fx.As(new(interfaces.UsersStore))),
			fx.Annotate(categoriesadapter.NewStore, fx.As(new(interfaces.CategoriesStore))),
			fx.Annotate(expensesadapter.NewStore, fx.As(new(interfaces.ExpensesStore))),
			fx.Annotate(expensesadapter.NewAttachmentsStore, fx.As(new(interfaces.AttachmentsStore))),
			fx.Annotate(templatesadapter.NewStore, fx.As(new(interfaces.TemplatesStore))),
			fx.Annotate(settlementsadapter.NewStore, fx.As(new(interfaces.SettlementsStore))),
			fx.Annotate(documentsadapter.NewStore, fx.As(new(interfaces.DocumentsStore))),
			fx.Annotate(alertsadapter.NewStore, fx.As(new(interfaces.AlertsStore))),
			fx.Annotate(pushadapter.NewStore, fx.As(new(interfaces.PushSubscriptionsStore))),
			fx.Annotate(metersadapter.NewStore, fx.As(new(interfaces.MetersStore))),
			fx.Annotate(authadapter.NewFirebaseProvisioner, fx.As(new(interfaces.AuthProvisioner))),

			home.New,
			users.New,
			func(u users.Usecases) interfaces.UsersService { return u },
			foyers.New,
			categories.New,
			alerts.New,
			// Adapters that bind alerts.Usecases to each consumer's
			// narrow AlertsHook interface — FX resolves by the parameter
			// type each constructor asks for.
			func(a alerts.Usecases) expenses.AlertsHook { return a },
			func(a alerts.Usecases) templates.AlertsHook { return a },
			func(a alerts.Usecases) settlements.AlertsHook { return a },
			func(a alerts.Usecases) meters.AlertsHook { return a },
			expenses.New,
			templates.New,
			settlements.New,
			documents.New,
			pushuc.New,
			meters.New,
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
