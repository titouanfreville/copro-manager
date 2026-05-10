package routes

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/titouanfreville/copro-manager/api/src/core/rest"
	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/contracts"
	routeerrors "github.com/titouanfreville/copro-manager/api/src/servers/api/routes/errors"
	"github.com/titouanfreville/copro-manager/api/src/servers/api/shared"
)

type societyRequest struct {
	Name    string `json:"name"`
	Phone   string `json:"phone,omitempty"`
	Email   string `json:"email,omitempty"`
	Website string `json:"website,omitempty"`
	Address string `json:"address,omitempty"`
}

type contactRequest struct {
	Name  string `json:"name,omitempty"`
	Role  string `json:"role,omitempty"`
	Phone string `json:"phone,omitempty"`
	Email string `json:"email,omitempty"`
}

type contractRequest struct {
	Name             string         `json:"name"`
	CategoryID       string         `json:"category_id"`
	Society          societyRequest `json:"society"`
	Contact          contactRequest `json:"contact,omitempty"`
	StartDate        string         `json:"start_date,omitempty"`
	EndDate          string         `json:"end_date,omitempty"`
	AmountCents      int            `json:"amount_cents,omitempty"`
	BillingFrequency string         `json:"billing_frequency,omitempty"`
	TemplateID       string         `json:"template_id,omitempty"`
	Status           string         `json:"status,omitempty"`
	Note             string         `json:"note,omitempty"`
}

func (req contractRequest) toInput(actorUID string) (contracts.CreateInput, error) {
	draft := entities.ContractDraft{
		Name:       req.Name,
		CategoryID: req.CategoryID,
		Society: entities.Society{
			Name:    req.Society.Name,
			Phone:   req.Society.Phone,
			Email:   req.Society.Email,
			Website: req.Society.Website,
			Address: req.Society.Address,
		},
		Contact: entities.Contact{
			Name:  req.Contact.Name,
			Role:  req.Contact.Role,
			Phone: req.Contact.Phone,
			Email: req.Contact.Email,
		},
		AmountCents:      req.AmountCents,
		BillingFrequency: entities.BillingFrequency(req.BillingFrequency),
		TemplateID:       req.TemplateID,
		Status:           entities.ContractStatus(req.Status),
		Note:             req.Note,
	}
	if req.StartDate != "" {
		t, err := parseDateOnly(req.StartDate)
		if err != nil {
			return contracts.CreateInput{}, entities.ValidationError{Key: "start_date", Message: "format attendu : YYYY-MM-DD"}
		}
		draft.StartDate = t
	}
	if req.EndDate != "" {
		t, err := parseDateOnly(req.EndDate)
		if err != nil {
			return contracts.CreateInput{}, entities.ValidationError{Key: "end_date", Message: "format attendu : YYYY-MM-DD"}
		}
		draft.EndDate = t
	}
	return contracts.CreateInput{ActorUserID: actorUID, ContractDraft: draft}, nil
}

// parseDateOnly accepts the strict YYYY-MM-DD form (matches the
// frontend `<input type="date">` output) and returns midnight UTC.
// Rejects out-of-range day/month values that Go's time.Parse would
// silently normalize (e.g. "2026-02-30" → 2026-03-02): re-formatting
// the parsed time and comparing to the original string catches every
// such normalization without an external lib.
func parseDateOnly(raw string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return t, err
	}
	if t.Format("2006-01-02") != raw {
		return time.Time{}, fmt.Errorf("invalid date %q", raw)
	}
	return t, nil
}

// ListContracts handles GET /contracts.
func (e *Endpoints) ListContracts(w http.ResponseWriter, r *http.Request) {
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	out, err := e.usecases.Contracts.List(r.Context(), actorUID)
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	if out == nil {
		out = []entities.Contract{}
	}
	rest.Render().JSON(http.StatusOK, w, r, out)
}

// CreateContract handles POST /contracts.
func (e *Endpoints) CreateContract(w http.ResponseWriter, r *http.Request) {
	var req contractRequest
	if err := rest.Bind().JSONData(r, &req); err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_BODY", "invalid JSON body"))
		return
	}
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	in, err := req.toInput(actorUID)
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	c, err := e.usecases.Contracts.Create(r.Context(), in)
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().JSON(http.StatusCreated, w, r, c)
}

// UpdateContract handles PATCH /contracts/{id}.
func (e *Endpoints) UpdateContract(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_ID", "missing contract id"))
		return
	}
	var req contractRequest
	if err := rest.Bind().JSONData(r, &req); err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_BODY", "invalid JSON body"))
		return
	}
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	in, err := req.toInput(actorUID)
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	c, err := e.usecases.Contracts.Update(r.Context(), id, in)
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().JSON(http.StatusOK, w, r, c)
}

// DeleteContract handles DELETE /contracts/{id}.
func (e *Endpoints) DeleteContract(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_ID", "missing contract id"))
		return
	}
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	if err := e.usecases.Contracts.Delete(r.Context(), id, actorUID); err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().NoContent(http.StatusNoContent, w)
}
