package api

import (
	"github.com/go-chi/chi/v5"

	"github.com/titouanfreville/copro-manager/api/src/servers/api/middlewares"
)

func (transport *API) initRoutes(r chi.Router) {
	r.Get("/", transport.endpoints.Home)
	r.Get("/uptime", transport.endpoints.Uptime)

	r.Route("/admin", func(adminRouter chi.Router) {
		adminRouter.Use(transport.middlewares.RequireAdminKey)
		adminRouter.Get("/foyers", transport.endpoints.AdminListFoyers)
		adminRouter.Post("/foyers", transport.endpoints.AdminCreateFoyer)
		adminRouter.Patch("/foyers/{id}", transport.endpoints.AdminUpdateFoyerParts)
		adminRouter.Post("/foyers/{id}/members", transport.endpoints.AdminAddFoyerMember)
		adminRouter.Post("/expenses/import", transport.endpoints.AdminImportExpenses)
		adminRouter.Post("/users/{id}/reset-password", transport.endpoints.AdminResetUserPassword)
	})

	// Foyer-facing routes — Bearer Firebase ID token required. Reads run
	// directly against Firestore from the SvelteKit app (auth-gated by the
	// rules in infra/firebase/firestore.rules); only mutations stay here so
	// the share-computation logic remains canonical.
	r.Group(func(authed chi.Router) {
		authed.Use(middlewares.RequireAuth)
		authed.Post("/expenses", transport.endpoints.CreateExpense)
	})

	r.NotFound(transport.endpoints.NotFound)
}
