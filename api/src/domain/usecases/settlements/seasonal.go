package settlements

import (
	"context"

	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/core/authz"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// seasonalResolver clears any non-resolved balance_seasonal alerts
// when the live ledger balance hits exactly zero. Mutations on
// settlements (Create/Update/Delete) trigger it as a best-effort
// follow-up — the cascade never blocks the originating mutation.
//
// Lives in its own file because the balance recomputation is its
// own concern and reads from both expenses and settlements stores
// — keeping it out of settlements.go preserves the orchestration
// file as pure CRUD.
type seasonalResolver struct {
	logger      *zap.Logger
	expenses    interfaces.ExpensesStore
	settlements interfaces.SettlementsStore
	foyers      interfaces.FoyersStore
	alerts      AlertsHook
}

func newSeasonalResolver(
	logger *zap.Logger,
	expenses interfaces.ExpensesStore,
	settlements interfaces.SettlementsStore,
	foyers interfaces.FoyersStore,
	alerts AlertsHook,
) *seasonalResolver {
	return &seasonalResolver{
		logger:      logger.Named("seasonal_resolver"),
		expenses:    expenses,
		settlements: settlements,
		foyers:      foyers,
		alerts:      alerts,
	}
}

// resolveIfZero recomputes the live balance and, when it lands at
// exactly zero, clears every non-resolved balance_seasonal alert.
// Best-effort — every leg fails silently with a warn log. The
// originating mutation has already committed by the time we run.
func (r *seasonalResolver) resolveIfZero(ctx context.Context) {
	if r.alerts == nil {
		return
	}
	net, ok := r.computeNet(ctx)
	if !ok || net != 0 {
		return
	}
	if err := r.alerts.ResolveSeasonalAll(ctx); err != nil {
		r.logger.Warn("seasonal-resolve failed", zap.Error(err))
	}
}

// computeNet sums the live balance across non-settled, non-pending
// expenses minus settlement transfers. Positive net → 1er owes RDC;
// negative → RDC owes 1er; zero → balanced. Returns false when any
// underlying read fails (resolver short-circuits).
func (r *seasonalResolver) computeNet(ctx context.Context) (int, bool) {
	expenses, err := r.expenses.List(ctx)
	if err != nil {
		r.logger.Warn("seasonal-resolve: expense list failed", zap.Error(err))
		return 0, false
	}
	settlements, err := r.settlements.List(ctx)
	if err != nil {
		r.logger.Warn("seasonal-resolve: settlement list failed", zap.Error(err))
		return 0, false
	}
	rdc, premier, err := authz.LoadBothFoyers(ctx, r.foyers)
	if err != nil {
		r.logger.Warn("seasonal-resolve: load foyers failed", zap.Error(err))
		return 0, false
	}
	net := 0
	for _, e := range expenses {
		if e.Settled || e.AmountPending {
			continue
		}
		switch e.PayerFoyerID {
		case rdc.ID:
			net += e.Share1erCents
		case premier.ID:
			net -= e.ShareRDCCents
		}
	}
	for _, s := range settlements {
		if s.FromFoyerID == premier.ID && s.ToFoyerID == rdc.ID {
			net -= s.AmountCents
		} else if s.FromFoyerID == rdc.ID && s.ToFoyerID == premier.ID {
			net += s.AmountCents
		}
	}
	return net, true
}
