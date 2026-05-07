package routes

import (
	"net/http"
	"time"

	"github.com/titouanfreville/copro-manager/api/src/core/rest"
	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/expenses"
	routeerrors "github.com/titouanfreville/copro-manager/api/src/servers/api/routes/errors"
	"github.com/titouanfreville/copro-manager/api/src/servers/api/shared"
)

type createExpenseRequest struct {
	Name             string `json:"name"`
	AmountCents      int    `json:"amount_cents"`
	Currency         string `json:"currency"`
	Date             string `json:"date"`
	PaymentDate      string `json:"payment_date,omitempty"`
	PayerFoyerID     string `json:"payer_foyer_id"`
	CategoryID       string `json:"category_id"`
	DistributionMode string `json:"distribution_mode"`
	ShareRDCCents    int    `json:"share_rdc_cents,omitempty"`
	Share1erCents    int    `json:"share_1er_cents,omitempty"`
	Settled          bool   `json:"settled,omitempty"`
	SettledAt        string `json:"settled_at,omitempty"`
	Note             string `json:"note,omitempty"`
}

// CreateExpense handles POST /expenses.
func (e *Endpoints) CreateExpense(w http.ResponseWriter, r *http.Request) {
	var req createExpenseRequest
	if err := rest.Bind().JSONData(r, &req); err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_BODY", "invalid JSON body"))
		return
	}

	date, err := parseDateOrNil(req.Date)
	if err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_DATE", "date must be RFC3339 or YYYY-MM-DD"))
		return
	}
	paymentDate, err := parseDateOrNilPtr(req.PaymentDate)
	if err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_DATE", "payment_date must be RFC3339 or YYYY-MM-DD"))
		return
	}
	settledAt, err := parseDateOrNilPtr(req.SettledAt)
	if err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_DATE", "settled_at must be RFC3339 or YYYY-MM-DD"))
		return
	}

	actorUID, _ := r.Context().Value(shared.UserID).(string)

	exp, err := e.usecases.Expenses.Create(r.Context(), expenses.CreateInput{
		ActorUserID:      actorUID,
		Name:             req.Name,
		AmountCents:      req.AmountCents,
		Currency:         req.Currency,
		Date:             date,
		PaymentDate:      paymentDate,
		PayerFoyerID:     req.PayerFoyerID,
		CategoryID:       req.CategoryID,
		DistributionMode: entities.DistributionMode(req.DistributionMode),
		ShareRDCCents:    req.ShareRDCCents,
		Share1erCents:    req.Share1erCents,
		Settled:          req.Settled,
		SettledAt:        settledAt,
		Note:             req.Note,
	})
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}

	rest.Render().JSON(http.StatusCreated, w, r, exp)
}

// parseDateOrNil accepts either RFC3339 or a bare YYYY-MM-DD. Empty input
// returns the zero time so the usecase's "required" validation kicks in.
func parseDateOrNil(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", s)
}

func parseDateOrNilPtr(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := parseDateOrNil(s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
