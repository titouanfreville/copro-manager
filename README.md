# copro-manager

Small tool to manage a French copropriété shared by 2 households.

## Layout

```
api/                  # Go HTTP API (Clean Architecture, Firestore-backed)
app/                  # SvelteKit PWA (Tailwind, Firebase Auth)
infra/                # OpenTofu modules (Cloud Run, Firestore, GCS, Firebase Hosting, WIF)
.github/workflows/    # CI/CD: api-pr, api-deploy, app-pr, app-deploy, infra-pr
```

## Stack

- **API**: Go 1.26 · `chi` · `fx` · `zap` · `firestore` · `firebase-admin` · `gocloud-storage`
- **App**: SvelteKit · Svelte 5 (runes) · TypeScript · Tailwind · `@vite-pwa/sveltekit`
- **Auth**: Firebase Auth (email/password)
- **Infra**: OpenTofu · GCP project `copro-manager` · region `europe-west9`
- **CI/CD**: GitHub Actions with Workload Identity Federation

## Local development

### Prerequisites

- Go 1.26+
- Node 22+
- OpenTofu (`brew install opentofu`) — only needed if you change infra
- `gcloud` CLI, logged in with Application Default Credentials:
  ```bash
  gcloud auth application-default login
  gcloud config set project copro-manager
  ```

### API

```bash
cd api
cp ../.env.sample .env
go mod download
go run ./bin/app          # listens on :8080
```

### App

```bash
cd app
cp .env.sample .env       # then fill PUBLIC_FIREBASE_API_KEY / PROJECT_ID / MESSAGING_SENDER_ID / APP_ID
npm install               # AUTH_DOMAIN and STORAGE_BUCKET are optional (default to <projectId>.firebaseapp.com / .firebasestorage.app)
npm run dev               # http://localhost:5173
```

### Local API auth bypass (dev shortcut)

The API requires a Firebase ID token by default. For local iteration without
Firebase Auth, set `middlewares.allow_bypass: true` and `middlewares.bypass_auth_key: <uuid>`
in `api/conf/local.yml`, then call the API with:

```
Authorization: Bypasses <key>
X-Bypass-User-ID: <uid-of-a-seeded-foyer-member>
```

`allow_bypass` and `bypass_auth_key` are **YAML config keys** (read from
`conf/*.yml`), not environment variables. They MUST stay disabled in deployed
configs (`allow_bypass: false`, `bypass_auth_key: ""`).

### Pre-PR checks

```bash
# api
cd api && golangci-lint run ./... && gosec ./... && go test ./src/...

# app
cd app && npm run check && npm run build

# infra
cd infra/terraform && tofu fmt -recursive -check && (cd env && tofu validate)
```

## First-time GCP setup

The bootstrap stack creates the terraform state bucket, the GitHub Actions service account and the Workload Identity Federation pool. Run it once:

```bash
cd infra/terraform/bootstrap
tofu init
tofu apply
../../../infra/scripts/setup-wif.sh   # prints the values to copy into GitHub secrets
```

Then provision the application stack:

```bash
cd ../env
tofu init
tofu apply
```

Add the printed `WIF_PROVIDER` and `WIF_SERVICE_ACCOUNT` plus your Firebase web app config keys (`PUBLIC_FIREBASE_API_KEY`, `PUBLIC_FIREBASE_PROJECT_ID`, `PUBLIC_FIREBASE_MESSAGING_SENDER_ID`, `PUBLIC_FIREBASE_APP_ID`) to repo secrets, and you're set.

## Deployment

- Push to `main` touching `api/**` → builds and deploys the API to Cloud Run
- Push to `main` touching `app/**` → builds and deploys the PWA to Firebase Hosting
- Infra changes are **applied manually** — PRs validate via `tofu fmt`/`validate` only

## Agent rules

See [`AGENTS.md`](./AGENTS.md) for the canonical architectural rules and locked decisions. Sub-folders may carry their own scoped `AGENTS.md`.
