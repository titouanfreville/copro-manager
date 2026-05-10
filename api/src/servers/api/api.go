package api

import (
	"context"
	"net/http"
	"time"

	fs "cloud.google.com/go/firestore"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/domain/usecases"
	"github.com/titouanfreville/copro-manager/api/src/servers"
	"github.com/titouanfreville/copro-manager/api/src/servers/api/config"
	"github.com/titouanfreville/copro-manager/api/src/servers/api/middlewares"
	"github.com/titouanfreville/copro-manager/api/src/servers/api/routes"
	"github.com/titouanfreville/copro-manager/api/src/servers/cors"
)

// API implements the servers.TCP interface for the HTTP transport layer.
type API struct {
	httpConfig  *config.Config
	endpoints   *routes.Endpoints
	httpServer  *http.Server
	middlewares *middlewares.Middlewares
}

// NewHTTP creates a new HTTP API transport.
func NewHTTP(httpConfig *config.Config, logger *zap.Logger, uc *usecases.Usecases, mw *middlewares.Middlewares, fsClient *fs.Client) servers.TCP {
	return &API{
		httpConfig:  httpConfig,
		endpoints:   routes.NewEndpoints(logger, uc, fsClient),
		middlewares: mw,
	}
}

func (transport *API) ListenAndServe() error {
	r := chi.NewRouter()
	transport.initMiddlewares(r)
	transport.initRoutes(r)

	transport.httpServer = &http.Server{
		Addr:              transport.httpConfig.GetAddress(),
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return transport.httpServer.ListenAndServe()
}

func (transport *API) Shutdown() error {
	return transport.httpServer.Shutdown(context.Background())
}

func (transport *API) GetAddress() string {
	return transport.httpConfig.GetAddress()
}

func (transport *API) initMiddlewares(router chi.Router) {
	router.Use(middleware.StripSlashes)
	router.Use(middleware.SetHeader("X-Frame-Options", "deny"))
	router.Use(middleware.Timeout(5 * time.Minute))
	router.Use(middleware.Heartbeat("/ping"))

	cors.InitGoCHI(transport.httpConfig.CORS, router)

	if transport.httpConfig.NoCache {
		router.Use(middleware.NoCache)
	}

	router.Use(middlewares.RequestID)
	router.Use(transport.middlewares.RequestLogger)
	router.Use(transport.middlewares.Authorize)
}
