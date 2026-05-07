package rest

import (
	"net/http"

	"github.com/go-chi/render"
)

var rend Renderer = RenderImpl{}

// RenderImpl implements the Renderer interface.
type RenderImpl struct{}

// Render provides a Renderer singleton.
func Render() Renderer {
	return rend
}

func (RenderImpl) JSON(status int, w http.ResponseWriter, r *http.Request, obj interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	render.JSON(w, r, obj)
}

func (RenderImpl) NoContent(status int, w http.ResponseWriter) {
	w.WriteHeader(status)
}
