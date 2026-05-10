package routes

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/titouanfreville/copro-manager/api/src/core/rest"
	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/templates"
	routeerrors "github.com/titouanfreville/copro-manager/api/src/servers/api/routes/errors"
	"github.com/titouanfreville/copro-manager/api/src/servers/api/shared"
)

type createTemplateRequest struct {
	Name               string `json:"name"`
	AmountDefaultCents int    `json:"amount_default_cents"`
	Currency           string `json:"currency"`
	CategoryID         string `json:"category_id"`
	PayerFoyerID       string `json:"payer_foyer_id"`
	DistributionMode   string `json:"distribution_mode"`
	ShareRDCCents      int    `json:"share_rdc_cents,omitempty"`
	Share1erCents      int    `json:"share_1er_cents,omitempty"`
	Note               string `json:"note,omitempty"`
	ScheduleActive     bool   `json:"schedule_active,omitempty"`
	Frequency          string `json:"frequency,omitempty"`
	DayOfMonth         int    `json:"day_of_month,omitempty"`
	StartDate          string `json:"start_date,omitempty"`
	EndDate            string `json:"end_date,omitempty"`
}

func (req createTemplateRequest) toInput(actorUID string) (templates.CreateTemplateInput, error) {
	startDate, err := parseDateOrNil(req.StartDate)
	if err != nil {
		return templates.CreateTemplateInput{}, err
	}
	endDate, err := parseDateOrNilPtr(req.EndDate)
	if err != nil {
		return templates.CreateTemplateInput{}, err
	}
	return templates.CreateTemplateInput{
		ActorUserID: actorUID,
		ExpenseTemplateDraft: entities.ExpenseTemplateDraft{
			Name:               req.Name,
			AmountDefaultCents: req.AmountDefaultCents,
			Currency:           req.Currency,
			CategoryID:         req.CategoryID,
			PayerFoyerID:       req.PayerFoyerID,
			DistributionMode:   entities.DistributionMode(req.DistributionMode),
			ShareRDCCents:      req.ShareRDCCents,
			Share1erCents:      req.Share1erCents,
			Note:               req.Note,
			ScheduleActive:     req.ScheduleActive,
			Frequency:          entities.Frequency(req.Frequency),
			DayOfMonth:         req.DayOfMonth,
			StartDate:          startDate,
			EndDate:            endDate,
		},
	}, nil
}

// ListTemplates handles GET /templates.
func (e *Endpoints) ListTemplates(w http.ResponseWriter, r *http.Request) {
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	out, err := e.usecases.Templates.List(r.Context(), actorUID)
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	if out == nil {
		out = []entities.ExpenseTemplate{}
	}
	rest.Render().JSON(http.StatusOK, w, r, out)
}

// CreateTemplate handles POST /templates.
func (e *Endpoints) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	var req createTemplateRequest
	if err := rest.Bind().JSONData(r, &req); err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_BODY", "invalid JSON body"))
		return
	}
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	in, err := req.toInput(actorUID)
	if err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_DATE", err.Error()))
		return
	}
	t, err := e.usecases.Templates.Create(r.Context(), in)
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().JSON(http.StatusCreated, w, r, t)
}

// UpdateTemplate handles PATCH /templates/{id}.
func (e *Endpoints) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_ID", "missing template id"))
		return
	}
	var req createTemplateRequest
	if err := rest.Bind().JSONData(r, &req); err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_BODY", "invalid JSON body"))
		return
	}
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	in, err := req.toInput(actorUID)
	if err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_DATE", err.Error()))
		return
	}
	t, err := e.usecases.Templates.Update(r.Context(), id, in)
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().JSON(http.StatusOK, w, r, t)
}

// DeleteTemplate handles DELETE /templates/{id}.
func (e *Endpoints) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_ID", "missing template id"))
		return
	}
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	if err := e.usecases.Templates.Delete(r.Context(), id, actorUID); err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().NoContent(http.StatusNoContent, w)
}

// MaterializeRecurring handles POST /expenses/materialize-recurring (foyer-
// authed). Same logic as AdminMaterializeRecurring — exposed to authed
// foyer members so the frontend can fire it on /expenses mount as a
// backstop to the daily cron. Idempotent. The actor UID is propagated to
// the usecase so non-foyer-members can't trigger materialization.
func (e *Endpoints) MaterializeRecurring(w http.ResponseWriter, r *http.Request) {
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	e.runMaterialize(w, r, actorUID)
}

// AdminMaterializeRecurring handles POST /admin/expense-templates/materialize-recurring.
// Idempotent — safe to invoke from Cloud Scheduler daily even when nothing's due.
// Empty actor short-circuits the foyer-membership check (the AdminKey gate
// at transport stands in).
func (e *Endpoints) AdminMaterializeRecurring(w http.ResponseWriter, r *http.Request) {
	e.runMaterialize(w, r, "")
}

func (e *Endpoints) runMaterialize(w http.ResponseWriter, r *http.Request, actorUID string) {
	summary, err := e.usecases.Templates.MaterializeRecurring(r.Context(), actorUID)
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().JSON(http.StatusOK, w, r, summary)
}
