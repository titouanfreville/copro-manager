package routes

import (
	"net/http"

	"github.com/titouanfreville/copro-manager/api/src/core/rest"
	routeerrors "github.com/titouanfreville/copro-manager/api/src/servers/api/routes/errors"
)

// NotFound handles requests to undefined routes.
func (e *Endpoints) NotFound(w http.ResponseWriter, r *http.Request) {
	rest.Render().JSON(http.StatusNotFound, w, r, routeerrors.NewServErrors("NOT_FOUND", "route not found"))
}
