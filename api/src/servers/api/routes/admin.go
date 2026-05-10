package routes

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	coprosadapter "github.com/titouanfreville/copro-manager/api/src/adapters/copros"
	"github.com/titouanfreville/copro-manager/api/src/core/rest"
	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/expenses"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/foyers"
	routeerrors "github.com/titouanfreville/copro-manager/api/src/servers/api/routes/errors"
)

// memberRequest is the JSON shape for picking-or-creating a member. The
// admin UI may send either user_id (existing) or email + display_name (new).
type memberRequest struct {
	UserID      string `json:"user_id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

func (m memberRequest) toInput() foyers.MemberInput {
	return foyers.MemberInput{
		UserID:      m.UserID,
		Email:       m.Email,
		DisplayName: m.DisplayName,
	}
}

type createFoyerRequest struct {
	Floor  string        `json:"floor"`
	Name   string        `json:"name"`
	Parts  int           `json:"parts"`
	Member memberRequest `json:"member"`
}

type createFoyerResponse struct {
	Foyer     entities.Foyer `json:"foyer"`
	ResetLink string         `json:"reset_link,omitempty"`
}

type listFoyersResponse struct {
	Foyers []foyers.ListedFoyer `json:"foyers"`
}

type addMemberResponse struct {
	Foyer     entities.Foyer `json:"foyer"`
	ResetLink string         `json:"reset_link,omitempty"`
}

// updatePartsRequest uses *int so we can distinguish missing field from
// an explicit zero. An empty body `{}` would otherwise decode Parts to 0,
// silently zeroing out the foyer's tantième share.
type updatePartsRequest struct {
	Parts *int `json:"parts"`
}

type resetPasswordResponse struct {
	ResetLink string `json:"reset_link"`
}

// importExpensesMaxBytes caps the multipart body so a stray upload can't
// pin the API. The legacy CSV is ~30 KB; 5 MB is generous.
const importExpensesMaxBytes int64 = 5 << 20

type importExpensesResponse struct {
	*expenses.ImportSummary
}

// AdminCreateFoyer handles POST /admin/foyers.
func (e *Endpoints) AdminCreateFoyer(w http.ResponseWriter, r *http.Request) {
	var req createFoyerRequest
	if err := rest.Bind().JSONData(r, &req); err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_BODY", "invalid JSON body"))
		return
	}

	result, err := e.usecases.Foyers.Create(r.Context(), foyers.CreateInput{
		Floor:  entities.FoyerFloor(req.Floor),
		Name:   req.Name,
		Parts:  req.Parts,
		Member: req.Member.toInput(),
	})
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}

	rest.Render().JSON(http.StatusCreated, w, r, createFoyerResponse{
		Foyer:     result.Foyer,
		ResetLink: result.ResetLink,
	})
}

// AdminListFoyers handles GET /admin/foyers — returns every foyer with its
// members enriched from our users store.
func (e *Endpoints) AdminListFoyers(w http.ResponseWriter, r *http.Request) {
	list, err := e.usecases.Foyers.List(r.Context())
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}

	rest.Render().JSON(http.StatusOK, w, r, listFoyersResponse{Foyers: list})
}

// AdminAddFoyerMember handles POST /admin/foyers/{id}/members.
func (e *Endpoints) AdminAddFoyerMember(w http.ResponseWriter, r *http.Request) {
	foyerID := chi.URLParam(r, "id")
	if foyerID == "" {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_ID", "missing foyer id"))
		return
	}

	var req memberRequest
	if err := rest.Bind().JSONData(r, &req); err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_BODY", "invalid JSON body"))
		return
	}

	result, err := e.usecases.Foyers.AddMember(r.Context(), foyerID, req.toInput())
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}

	rest.Render().JSON(http.StatusCreated, w, r, addMemberResponse{
		Foyer:     result.Foyer,
		ResetLink: result.ResetLink,
	})
}

// AdminUpdateFoyerParts handles PATCH /admin/foyers/{id}.
func (e *Endpoints) AdminUpdateFoyerParts(w http.ResponseWriter, r *http.Request) {
	foyerID := chi.URLParam(r, "id")
	if foyerID == "" {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_ID", "missing foyer id"))
		return
	}

	var req updatePartsRequest
	if err := rest.Bind().JSONData(r, &req); err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_BODY", "invalid JSON body"))
		return
	}
	if req.Parts == nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("VALIDATION_ERROR", "parts is required"))
		return
	}

	if err := e.usecases.Foyers.UpdateParts(r.Context(), foyerID, *req.Parts); err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}

	rest.Render().NoContent(http.StatusNoContent, w)
}

// AdminImportExpenses handles POST /admin/expenses/import — multipart upload
// of the legacy CSV (the spreadsheet the household has been keeping by hand).
// Form fields:
//   - file:           the CSV (required)
//   - payer_foyer_id: which foyer paid the imported expenses (required, since
//     the legacy CSV doesn't track payer identity)
func (e *Endpoints) AdminImportExpenses(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, importExpensesMaxBytes)
	if err := r.ParseMultipartForm(importExpensesMaxBytes); err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_BODY", "invalid multipart body"))
		return
	}

	payerFoyerID := strings.TrimSpace(r.FormValue("payer_foyer_id"))
	if payerFoyerID == "" {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("VALIDATION_ERROR", "payer_foyer_id is required"))
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("VALIDATION_ERROR", "file is required"))
		return
	}
	defer func() { _ = file.Close() }()

	summary, err := e.usecases.Expenses.ImportCSV(r.Context(), file, payerFoyerID)
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}

	rest.Render().JSON(http.StatusOK, w, r, importExpensesResponse{ImportSummary: summary})
}

// consolidateCoprosRequest is the optional body for the consolidation
// endpoint. When the foyer-consensus auto-detect succeeds the body can
// be empty; otherwise the operator passes an explicit canonical id.
type consolidateCoprosRequest struct {
	CanonicalCoproID string `json:"canonical_copro_id,omitempty"`
	DryRun           bool   `json:"dry_run,omitempty"`
}

// AdminConsolidateCopros handles POST /admin/copros/consolidate. One-shot
// data-fix: rewrites every dependent collection's `copro_id` to point at
// the canonical Copro doc and deletes the now-orphan rows. Returns
// per-collection rewrite counts.
func (e *Endpoints) AdminConsolidateCopros(w http.ResponseWriter, r *http.Request) {
	var req consolidateCoprosRequest
	// Empty body is fine: the request type is intentionally optional.
	if r.ContentLength > 0 {
		if err := rest.Bind().JSONData(r, &req); err != nil {
			rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_BODY", "invalid JSON body"))
			return
		}
	}

	summary, err := coprosadapter.Consolidate(r.Context(), e.firestore, e.logger, coprosadapter.ConsolidationOptions{
		CanonicalCoproIDOverride: strings.TrimSpace(req.CanonicalCoproID),
		DryRun:                   req.DryRun,
	})
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}

	rest.Render().JSON(http.StatusOK, w, r, summary)
}

// AdminResetUserPassword handles POST /admin/users/{id}/reset-password.
func (e *Endpoints) AdminResetUserPassword(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_ID", "missing user id"))
		return
	}

	link, err := e.usecases.Users.ResetPassword(r.Context(), userID)
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}

	rest.Render().JSON(http.StatusOK, w, r, resetPasswordResponse{ResetLink: link})
}
