package usecases

import (
	"time"

	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/categories"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/expenses"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/foyers"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/home"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/templates"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/users"
)

// Usecases aggregates all domain usecases.
type Usecases struct {
	logger     *zap.Logger
	since      time.Time
	Home       home.Usecases
	Foyers     foyers.Usecases
	Users      users.Usecases
	Categories categories.Usecases
	Expenses   expenses.Usecases
	Templates  templates.Usecases
}

// New creates the root usecases aggregator.
func New(
	logger *zap.Logger,
	initHome home.Usecases,
	initFoyers foyers.Usecases,
	initUsers users.Usecases,
	initCategories categories.Usecases,
	initExpenses expenses.Usecases,
	initTemplates templates.Usecases,
) *Usecases {
	return &Usecases{
		logger:     logger.Named("usecases"),
		since:      time.Now(),
		Home:       initHome,
		Foyers:     initFoyers,
		Users:      initUsers,
		Categories: initCategories,
		Expenses:   initExpenses,
		Templates:  initTemplates,
	}
}

// GetAppUptime returns the duration since the usecases were initialized.
func (uc *Usecases) GetAppUptime() time.Duration {
	return time.Since(uc.since)
}
