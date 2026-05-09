package routes

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/titouanfreville/copro-manager/api/src/core/rest"
	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/categories"
	routeerrors "github.com/titouanfreville/copro-manager/api/src/servers/api/routes/errors"
	"github.com/titouanfreville/copro-manager/api/src/servers/api/shared"
)

type createCategoryRequest struct {
	Name                    string `json:"name"`
	DefaultDistributionMode string `json:"default_distribution_mode,omitempty"`
}

type updateCategoryRequest struct {
	Name                    string `json:"name,omitempty"`
	DefaultDistributionMode string `json:"default_distribution_mode,omitempty"`
}

// CreateCategory handles POST /categories.
func (e *Endpoints) CreateCategory(w http.ResponseWriter, r *http.Request) {
	var req createCategoryRequest
	if err := rest.Bind().JSONData(r, &req); err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_BODY", "invalid JSON body"))
		return
	}
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	c, err := e.usecases.Categories.Create(r.Context(), categories.CreateCategoryInput{
		ActorUserID:             actorUID,
		Name:                    req.Name,
		DefaultDistributionMode: entities.DistributionMode(req.DefaultDistributionMode),
	})
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().JSON(http.StatusCreated, w, r, c)
}

// UpdateCategory handles PATCH /categories/{id}.
func (e *Endpoints) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_ID", "missing category id"))
		return
	}
	var req updateCategoryRequest
	if err := rest.Bind().JSONData(r, &req); err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_BODY", "invalid JSON body"))
		return
	}
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	c, err := e.usecases.Categories.Update(r.Context(), id, categories.UpdateCategoryInput{
		ActorUserID:             actorUID,
		Name:                    req.Name,
		DefaultDistributionMode: entities.DistributionMode(req.DefaultDistributionMode),
	})
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().JSON(http.StatusOK, w, r, c)
}

// DeleteCategory handles DELETE /categories/{id}.
func (e *Endpoints) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_ID", "missing category id"))
		return
	}
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	if err := e.usecases.Categories.Delete(r.Context(), id, actorUID); err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().NoContent(http.StatusNoContent, w)
}
