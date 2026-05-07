---
description: Add a new method to an existing domain in api/ with route handler and tests
arguments:
  - name: domain_name
    description: Existing domain (e.g. "expense", "document")
    required: true
  - name: method_name
    description: Method to add (e.g. "MarkPaid", "Approve")
    required: true
  - name: description
    description: One-line description of what the method does
    required: true
---

Add `$ARGUMENTS.method_name` to the `$ARGUMENTS.domain_name` domain in `api/`.

> **Naming:** convert manually to:
> - `<MethodPascal>` — PascalCase method name on the `Usecases` interface (e.g. `mark_paid` → `MarkPaid`)
> - `<methodCamel>` — camelCase if used for an internal helper
> - `<domain>` — snake_case from the first argument
>
> The slash-command runtime does NOT auto-transform argument case.

## What to do

### 1. Extend the usecase

`api/src/domain/usecases/<domain>/<domain>.go`

Add to the `Usecases` interface:
```go
<MethodPascal>(ctx context.Context, args...) (result, error)
```

Implement following the logging pattern:
```go
func (uc *usecases) <MethodPascal>(ctx context.Context, arg1 string) (string, error) {
    log := uc.logger.With(zap.String("method", "<MethodPascal>"), zap.String("arg1", arg1))

    // ... business logic ($ARGUMENTS.description) ...

    log.Info("Success")
    return result, nil
}
```

**Logging rules:**
- Bind `method=` and all relevant args at method start.
- `Info("Success")` on the happy path.
- `Warn` for expected failures (not found, validation, known edge cases).
- `Error` for unexpected critical failures.
- If the method has no I/O and cannot fail, drop the error return — bind, log success, return.

### 2. Extend the store interface and adapter (only if I/O is needed)

- Add the method to `api/src/domain/interfaces/<domain>_store.go`
- Implement it in `api/src/adapters/<domain>/firestore.go` (Firestore is the only datastore)

### 3. Add a route handler

In `api/src/servers/api/routes/<domain>.go`:
- Thin handler on the existing `Endpoints` struct
- Parse with `rest.Bind()`, call usecase, `ManageErrors()`, render with `rest.Render().JSON()`

### 4. Register the route

In `api/src/servers/api/routes.go`, add the new endpoint inside the existing `r.Route("/<domain>", ...)` block.

### 5. Tests

- Add a GoConvey test in `api/src/domain/usecases/<domain>/<domain>_test.go`
- Add a scenario in `api/tests/api/features/<domain>.feature`

## After

Run `cd api && go test ./src/... && golangci-lint run ./... && gosec ./...` and fix any issues.
