package routes

import (
	"net/http"

	"github.com/titouanfreville/copro-manager/api/src/core/rest"
)

type homeResponse struct {
	Message string `json:"message"`
}

// Home handles the GET / endpoint.
func (e *Endpoints) Home(w http.ResponseWriter, r *http.Request) {
	message := e.usecases.Home.Hello(r.Context())

	rest.Render().JSON(http.StatusOK, w, r, homeResponse{Message: message})
}

type uptimeResponse struct {
	Uptime string `json:"uptime"`
}

// Uptime handles the GET /uptime endpoint.
func (e *Endpoints) Uptime(w http.ResponseWriter, r *http.Request) {
	uptime := e.usecases.GetAppUptime()

	rest.Render().JSON(http.StatusOK, w, r, uptimeResponse{Uptime: uptime.String()})
}
