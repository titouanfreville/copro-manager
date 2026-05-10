package routes

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/titouanfreville/copro-manager/api/src/core/rest"
	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/settlements"
	routeerrors "github.com/titouanfreville/copro-manager/api/src/servers/api/routes/errors"
	"github.com/titouanfreville/copro-manager/api/src/servers/api/shared"
)

type createSettlementRequest struct {
	FromFoyerID string   `json:"from_foyer_id"`
	ToFoyerID   string   `json:"to_foyer_id"`
	AmountCents int      `json:"amount_cents"`
	Currency    string   `json:"currency,omitempty"`
	Date        string   `json:"date"`
	Note        string   `json:"note,omitempty"`
	ExpenseIDs  []string `json:"expense_ids,omitempty"`
}

func (req createSettlementRequest) toInput(actorUID string) (settlements.CreateInput, error) {
	date, err := parseDateOrNil(req.Date)
	if err != nil {
		return settlements.CreateInput{}, err
	}
	return settlements.CreateInput{
		ActorUserID: actorUID,
		SettlementDraft: entities.SettlementDraft{
			FromFoyerID: req.FromFoyerID,
			ToFoyerID:   req.ToFoyerID,
			AmountCents: req.AmountCents,
			Currency:    req.Currency,
			Date:        date,
			Note:        req.Note,
			ExpenseIDs:  req.ExpenseIDs,
		},
	}, nil
}

// ListSettlements handles GET /settlements.
func (e *Endpoints) ListSettlements(w http.ResponseWriter, r *http.Request) {
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	out, err := e.usecases.Settlements.List(r.Context(), actorUID)
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	if out == nil {
		out = []entities.Settlement{}
	}
	rest.Render().JSON(http.StatusOK, w, r, out)
}

// CreateSettlement handles POST /settlements.
func (e *Endpoints) CreateSettlement(w http.ResponseWriter, r *http.Request) {
	var req createSettlementRequest
	if err := rest.Bind().JSONData(r, &req); err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_BODY", "invalid JSON body"))
		return
	}
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	in, err := req.toInput(actorUID)
	if err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_DATE", "date must be RFC3339 or YYYY-MM-DD"))
		return
	}
	s, err := e.usecases.Settlements.Create(r.Context(), in)
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().JSON(http.StatusCreated, w, r, s)
}

// UpdateSettlement handles PATCH /settlements/{id}.
func (e *Endpoints) UpdateSettlement(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_ID", "missing settlement id"))
		return
	}
	var req createSettlementRequest
	if err := rest.Bind().JSONData(r, &req); err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_BODY", "invalid JSON body"))
		return
	}
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	in, err := req.toInput(actorUID)
	if err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_DATE", "date must be RFC3339 or YYYY-MM-DD"))
		return
	}
	s, err := e.usecases.Settlements.Update(r.Context(), id, in)
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().JSON(http.StatusOK, w, r, s)
}

// DeleteSettlement handles DELETE /settlements/{id}.
func (e *Endpoints) DeleteSettlement(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_ID", "missing settlement id"))
		return
	}
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	if err := e.usecases.Settlements.Delete(r.Context(), id, actorUID); err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().NoContent(http.StatusNoContent, w)
}
