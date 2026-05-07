package cors

import (
	"github.com/go-chi/chi/v5"
	gocors "github.com/go-chi/cors"
)

// InitGoCHI applies CORS middleware to the given chi router.
func InitGoCHI(conf Config, router chi.Router) {
	router.Use(gocors.Handler(gocors.Options{
		AllowedOrigins:   conf.AllowedOrigins,
		AllowedMethods:   conf.AllowedMethods,
		AllowedHeaders:   conf.AllowedHeaders,
		AllowCredentials: conf.AllowCredentials,
		MaxAge:           int(conf.MaxAge.Seconds()),
	}))
}
