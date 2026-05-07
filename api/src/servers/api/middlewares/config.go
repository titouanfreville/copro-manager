package middlewares

// Config holds the auth-related middleware settings.
//
// AllowBypass + BypassAuthKey enable a "Bypasses <key>" Authorization header
// that skips Firebase token verification — local-dev only, must be off in
// deployed environments.
//
// AdminAPIKey is the global shared secret gating the /admin/* subtree via
// "AdminKey <key>". An empty value disables every admin endpoint at the
// middleware layer; populate it via Secret Manager → Cloud Run env in prod.
type Config struct {
	AllowBypass   bool   `yaml:"allow_bypass"`
	BypassAuthKey string `yaml:"bypass_auth_key"`
	AdminAPIKey   string `yaml:"admin_api_key"`
}
