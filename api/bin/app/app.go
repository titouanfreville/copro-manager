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
	alertsstore "github.com/titouanfreville/copro-manager/api/src/adapters/store/alerts"
	categoriesstore "github.com/titouanfreville/copro-manager/api/src/adapters/store/categories"
	contractsstore "github.com/titouanfreville/copro-manager/api/src/adapters/store/contracts"
	coprosstore "github.com/titouanfreville/copro-manager/api/src/adapters/store/copros"
	documentsstore "github.com/titouanfreville/copro-manager/api/src/adapters/store/documents"
	expensesstore "github.com/titouanfreville/copro-manager/api/src/adapters/store/expenses"
	foyersstore "github.com/titouanfreville/copro-manager/api/src/adapters/store/foyers"
	metersstore "github.com/titouanfreville/copro-manager/api/src/adapters/store/meters"
	pushstore "github.com/titouanfreville/copro-manager/api/src/adapters/store/push"
	settlementsstore "github.com/titouanfreville/copro-manager/api/src/adapters/store/settlements"
	templatesstore "github.com/titouanfreville/copro-manager/api/src/adapters/store/templates"
	usersstore "github.com/titouanfreville/copro-manager/api/src/adapters/store/users"
	aiusagestore "github.com/titouanfreville/copro-manager/api/src/adapters/store/aiusage"
	validatorsadapter "github.com/titouanfreville/copro-manager/api/src/adapters/validators"
	"github.com/titouanfreville/copro-manager/api/src/core/config"
	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/alerts"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/categories"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/contracts"
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
	geminisvc "github.com/titouanfreville/copro-manager/api/src/services/gemini"
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
				aiusagestore.NewStore,
				fx.As(new(interfaces.AIUsageStore)),
			),
			fx.Annotate(
				func(lc fx.Lifecycle, cfg *config.Config, usage interfaces.AIUsageStore) (*geminisvc.Client, error) {
					c, err := geminisvc.NewClient(cfg.Gemini, usage)
					if err != nil {
						return nil, err
					}
					// Register Close on shutdown for symmetry with other GCP
					// service wrappers — currently a no-op on the genai SDK.
					lc.Append(fx.Hook{
						OnStop: func(_ context.Context) error { return c.Close() },
					})
					return c, nil
				},
				fx.As(new(interfaces.MeterReader)),
			),

			firebase.NewApp,
			firebase.NewAuthClient,
			firebase.NewAdminClient,

			fx.Annotate(foyersstore.NewStore, fx.As(new(interfaces.FoyersStore))),
			fx.Annotate(coprosstore.NewStore, fx.As(new(interfaces.CoprosStore))),
			fx.Annotate(usersstore.NewStore, fx.As(new(interfaces.UsersStore))),
			fx.Annotate(categoriesstore.NewStore, fx.As(new(interfaces.CategoriesStore))),
			fx.Annotate(expensesstore.NewStore, fx.As(new(interfaces.ExpensesStore))),
			fx.Annotate(expensesstore.NewAttachmentsStore, fx.As(new(interfaces.AttachmentsStore))),
			fx.Annotate(templatesstore.NewStore, fx.As(new(interfaces.TemplatesStore))),
			fx.Annotate(settlementsstore.NewStore, fx.As(new(interfaces.SettlementsStore))),
			fx.Annotate(documentsstore.NewStore, fx.As(new(interfaces.DocumentsStore))),
			fx.Annotate(contractsstore.NewStore, fx.As(new(interfaces.ContractsStore))),
			fx.Annotate(alertsstore.NewStore, fx.As(new(interfaces.AlertsStore))),
			fx.Annotate(pushstore.NewStore, fx.As(new(interfaces.PushSubscriptionsStore))),
			fx.Annotate(metersstore.NewStore, fx.As(new(interfaces.MetersStore))),
			fx.Annotate(validatorsadapter.NewContracts, fx.As(new(interfaces.ContractValidator))),
			fx.Annotate(validatorsadapter.NewDocuments, fx.As(new(interfaces.DocumentValidator))),
			fx.Annotate(validatorsadapter.NewSettlements, fx.As(new(interfaces.SettlementValidator))),
			fx.Annotate(validatorsadapter.NewTemplates, fx.As(new(interfaces.TemplateValidator))),
			fx.Annotate(validatorsadapter.NewExpenses, fx.As(new(interfaces.ExpenseValidator))),
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
			func(d documents.Usecases) expenses.DocumentsHook { return d },
			func(a alerts.Usecases) contracts.AlertsHook { return a },
			// Bridge expenses.Usecases → templates.ExpensesHook so the
			// templates package stays a leaf (no usecase-to-usecase
			// import). The hook accepts a draft; we build a CreateInput
			// here at the composition root.
			func(e expenses.Usecases) templates.ExpensesHook {
				return &templatesExpensesAdapter{u: e}
			},
			expenses.New,
			templates.New,
			settlements.New,
			documents.New,
			contracts.New,
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
		// Best-effort runtime migration: collapse the legacy
		// `expenses/{id}/attachments/{aid}` subcollection into top-level
		// `documents/{aid}` rows with `linked_expense_id` set. Idempotent
		// and safe to re-run on every boot — it stops touching rows once
		// they're all migrated. A failure is logged but doesn't abort
		// startup: the API still serves correctly while old subdocs
		// linger.
		fx.Invoke(
			func(client *fs.Client, logger *uberzap.Logger) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()
				if err := documentsstore.MigrateAttachmentsToDocuments(ctx, client, logger); err != nil {
					logger.Named("bootstrap").Warn("attachments→documents migration failed (continuing)", uberzap.Error(err))
				}
			},
		),
		fx.Invoke(api.Run),
	)

	fxapp.Start(app, timeout)
	<-app.Done()
	fxapp.Shutdown(app, timeout)
}

// templatesExpensesAdapter bridges the wide expenses.Usecases to the
// narrow templates.ExpensesHook so the templates package doesn't have
// to import the expenses package. Lives here at the composition root
// rather than in either domain — both packages stay leaves.
type templatesExpensesAdapter struct{ u expenses.Usecases }

func (a *templatesExpensesAdapter) Create(ctx context.Context, d entities.ExpenseDraft) (*entities.Expense, error) {
	return a.u.Create(ctx, expenses.CreateInput{ExpenseDraft: d})
}
