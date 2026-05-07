---
description: Scaffold a new domain in api/ with usecase, Firestore adapter, routes, FX wiring, and tests
arguments:
  - name: domain_name
    description: Snake_case name of the domain to create (e.g. "expense", "document", "vote")
    required: true
---

Scaffold a new domain `$ARGUMENTS.domain_name` in `api/` following the project's Clean Architecture rules.

> **Naming:** the argument is in snake_case. You convert manually to:
> - `<DomainPascal>` — PascalCase (e.g. `expense` → `Expense`, `meter_reading` → `MeterReading`)
> - `<domain>` — snake_case as provided (used for files, paths, collections, route prefixes)
>
> The slash-command runtime does NOT auto-transform argument case.

> **Module path:** read it from `api/go.mod`. Do NOT hard-code; the project module path is the single source of truth.

## What to create

### 1. Entity: `api/src/domain/entities/<domain>.go`

- Exported struct `<DomainPascal>` with `json:"snake_case"` tags **only**
- Domain entities are **storage-agnostic**: no `firestore:"…"`, `db:"…"` or other adapter tags
- Include an `ID` field (string, UUID v4)
- If the entity needs persistence shape divergence, define a private adapter-side struct in step 3 with `firestore:"…"` tags and map to/from the domain entity

### 2. Store interface: `api/src/domain/interfaces/<domain>_store.go`

```go
package interfaces

import (
    "context"
    "<module-path>/src/domain/entities"
)

type <DomainPascal>Store interface {
    Get(ctx context.Context, id string) (entities.<DomainPascal>, error)
    List(ctx context.Context) ([]entities.<DomainPascal>, error)
    Create(ctx context.Context, item entities.<DomainPascal>) error
    Update(ctx context.Context, item entities.<DomainPascal>) error
    Delete(ctx context.Context, id string) error
}
```

### 3. Firestore adapter: `api/src/adapters/<domain>/firestore.go`

- Implements the store interface above using `*firestore.Client`
- Define a private struct `<domain>Doc` here with `firestore:"…"` tags; map to/from `entities.<DomainPascal>`
- Collection name: `<domain>` (snake_case, plural if natural)
- Map `domainerrors.ErrNotFound` from `status.Code(err) == codes.NotFound`
- Constructor `NewStore(client *firestore.Client) interfaces.<DomainPascal>Store`

### 4. Usecase: `api/src/domain/usecases/<domain>/<domain>.go`

- Exported `Usecases` interface
- Unexported `usecases` struct depending on the store interface + `*zap.Logger`
- Constructor `New(logger *zap.Logger, store interfaces.<DomainPascal>Store) Usecases`
- Logger named `usecases.<domain>`
- Standard CRUD methods following the logging pattern:
  ```go
  log := uc.logger.With(zap.String("method", "Create"), zap.String("id", item.ID))
  // ... logic ...
  log.Info("Success")
  ```

### 5. Routes: `api/src/servers/api/routes/<domain>.go`

- Add handler methods on the existing `Endpoints` struct
- Thin handlers: `rest.Bind()` for input, call usecase, `ManageErrors()` on error, `rest.Render().JSON()` on success

### 6. Wire routes: `api/src/servers/api/routes.go`

```go
r.Route("/<domain>", func(r chi.Router) {
    r.Use(middlewares.RequireAuth) // For user-facing domains.
    // For admin-only domains, use middlewares.RequireAdminKey instead and
    // mount under /admin/<domain>.
    r.Get("/", transport.endpoints.List<DomainPascal>)
    r.Post("/", transport.endpoints.Create<DomainPascal>)
    r.Get("/{id}", transport.endpoints.Get<DomainPascal>)
    r.Put("/{id}", transport.endpoints.Update<DomainPascal>)
    r.Delete("/{id}", transport.endpoints.Delete<DomainPascal>)
})
```

### 7. Register in root usecases: `api/src/domain/usecases/usecases.go`

- Add a field for the new domain's `Usecases` interface
- Accept it in `New()`

### 8. Wire FX: `api/bin/app/app.go`

- Provide the store constructor
- Provide the usecase constructor
- Make sure FX can resolve everything

### 9. Tests

- `api/src/domain/usecases/<domain>/<domain>_test.go` — GoConvey unit tests with a mocked store and `zap.NewNop()`
- `api/tests/api/features/<domain>.feature` — Gherkin scenarios for the endpoints

## Rules

- Follow patterns from existing domains exactly
- All dependencies must have an interface for mocking
- Named logger with domain name
- Thin handlers — no business logic in route handlers
- **Datastore is Firestore only** — never scaffold pgx/sqlx/SQL adapters
- Run `cd api && go test ./src/... && golangci-lint run ./... && gosec ./...` after scaffolding
