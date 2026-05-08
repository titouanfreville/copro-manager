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
		// Cloud Scheduler hits this daily. Idempotent.
		adminRouter.Post("/expense-templates/materialize-recurring", transport.endpoints.AdminMaterializeRecurring)
	})

	// Foyer-facing routes — Bearer Firebase ID token required. Reads run
	// directly against Firestore from the SvelteKit app (auth-gated by the
	// rules in infra/firebase/firestore.rules); only mutations stay here so
	// the share-computation logic remains canonical.
	r.Group(func(authed chi.Router) {
		authed.Use(middlewares.RequireAuth)
		authed.Post("/expenses", transport.endpoints.CreateExpense)
		authed.Patch("/expenses/{id}", transport.endpoints.UpdateExpense)
		authed.Delete("/expenses/{id}", transport.endpoints.DeleteExpense)

		authed.Post("/expenses/{id}/attachments/upload-url", transport.endpoints.RequestAttachmentUploadURL)
		authed.Post("/expenses/{id}/attachments", transport.endpoints.RecordAttachment)
		authed.Get("/expenses/{id}/attachments/{attID}/download-url", transport.endpoints.GetAttachmentDownloadURL)
		authed.Delete("/expenses/{id}/attachments/{attID}", transport.endpoints.DeleteAttachment)

		authed.Get("/templates", transport.endpoints.ListTemplates)
		authed.Post("/templates", transport.endpoints.CreateTemplate)
		authed.Patch("/templates/{id}", transport.endpoints.UpdateTemplate)
		authed.Delete("/templates/{id}", transport.endpoints.DeleteTemplate)
		// Lazy materialization — frontend fires on /expenses mount as a
		// backstop to the daily Cloud Scheduler cron.
		authed.Post("/expenses/materialize-recurring", transport.endpoints.MaterializeRecurring)
	})

	r.NotFound(transport.endpoints.NotFound)
}
