package routes

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/titouanfreville/copro-manager/api/src/core/rest"
	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	routeerrors "github.com/titouanfreville/copro-manager/api/src/servers/api/routes/errors"
	"github.com/titouanfreville/copro-manager/api/src/servers/api/shared"
)

// ListAlerts handles GET /alerts.
func (e *Endpoints) ListAlerts(w http.ResponseWriter, r *http.Request) {
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	out, err := e.usecases.Alerts.List(r.Context(), actorUID)
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	if out == nil {
		out = []entities.Alert{}
	}
	rest.Render().JSON(http.StatusOK, w, r, out)
}

// MarkAlertRead handles POST /alerts/{id}/read.
func (e *Endpoints) MarkAlertRead(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	if err := e.usecases.Alerts.MarkRead(r.Context(), id, actorUID); err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().NoContent(http.StatusNoContent, w)
}

// DismissAlert handles POST /alerts/{id}/dismiss.
func (e *Endpoints) DismissAlert(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	if err := e.usecases.Alerts.Dismiss(r.Context(), id, actorUID); err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().NoContent(http.StatusNoContent, w)
}

// MarkAllAlertsRead handles POST /alerts/mark-all-read.
func (e *Endpoints) MarkAllAlertsRead(w http.ResponseWriter, r *http.Request) {
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	if err := e.usecases.Alerts.MarkAllRead(r.Context(), actorUID); err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().NoContent(http.StatusNoContent, w)
}

// AdminScanAlerts handles POST /admin/alerts/scan — Cloud Scheduler target.
func (e *Endpoints) AdminScanAlerts(w http.ResponseWriter, r *http.Request) {
	summary, err := e.usecases.Alerts.ScanTimeBased(r.Context())
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().JSON(http.StatusOK, w, r, struct {
		MissingReceiptFired int       `json:"missing_receipt_fired"`
		SeasonalFired       int       `json:"seasonal_fired"`
		At                  time.Time `json:"at"`
	}{
		MissingReceiptFired: summary.MissingReceiptFired,
		SeasonalFired:       summary.SeasonalFired,
		At:                  time.Now().UTC(),
	})
}
