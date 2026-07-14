# Stage 3 — Auth Integration Notes

Everything here is the wiring that connects the middleware/handlers files
into what you already built in Stage 1-2. Nothing below should require
touching your DB schema — `users.supabase_uid` and `users.role` should
already exist from your Stage 2 migration.

## 0. Why there's no JWT secret in this version

Since October 2025, new Supabase projects default to **asymmetric JWTs**
(ES256) instead of a shared HS256 secret. Supabase deliberately makes the
signing key non-extractable on this model — there's no secret in the
dashboard to copy, by design, not by mistake. The supported way to verify
tokens server-side is against the project's public JWKS endpoint:

```
<your-supabase-url>/auth/v1/.well-known/jwks.json
```

Good news: this needs **no new env var**. You already have
`SUPABASE_URL` from Stage 1's config work — `NewJWKSVerifier` just appends
the well-known path onto it.

(If your project is old enough to still be on the legacy shared-secret
model, that path still exists and still works. If it 404s, you're
confirmed on the new asymmetric model, which is what this code assumes.)

## 1. Dependencies

```bash
go get github.com/golang-jwt/jwt/v5
go get gorm.io/driver/sqlite  # test-only, for the in-memory test DB
go get github.com/stretchr/testify
```

No JWKS library needed — `jwks.go` fetches and parses the EC/P-256 keys
directly with the standard library, so there's no extra dependency whose
API might shift under you.

## 2. Config — nothing new required

If you don't already store `SUPABASE_URL` in your config struct from
Stage 1, add it now (you almost certainly need it anyway for the Supabase
client elsewhere):

```go
SupabaseURL string `mapstructure:"SUPABASE_URL"`
```

```
# .env
SUPABASE_URL=https://your-project-ref.supabase.co
```

## 3. Build the verifier once, at startup

`JWKSVerifier` caches keys in memory and refreshes them itself — build one
instance in your app container / `main.go`, not per-request:

```go
verifier := middleware.NewJWKSVerifier(appContainer.Config.SupabaseURL)
```

## 4. Router wiring

```go
api := router.Group("/api")

// Stage 3 exit-check routes
handlers.RegisterAuthTestRoutes(api, appContainer.DB, verifier)

// Pattern for every real Stage 4 endpoint going forward:
protected := api.Group("/students")
protected.Use(middleware.SupabaseAuth(appContainer.DB, verifier))
protected.POST("", middleware.RequireRole(models.RolePrincipal), studentHandlers.Create)
protected.GET("/:id", studentHandlers.Get) // any logged-in role, no RequireRole needed
```

## 5. Swagger security setup (item 19)

Add the security definition once, near your other `@title`/`@version`
annotations:

```go
// @securitydefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and the Supabase access token.
```

Then any handler behind `SupabaseAuth` gets one line added to its existing
swaggo comment block:

```go
// @Security BearerAuth
```

(Already on the two test handlers in `auth_test_routes.go` — copy that
pattern onto every Stage 4 handler as you build it.)

```bash
swag init
```

Protected routes should now show a lock icon and an "Authorize" button in
the Swagger UI.

## 6. Automated exit check

```bash
go test ./internal/middleware/...
```

Covers: missing token → 401, expired token → 401, valid token but no
matching `users` row → 403, valid token + matching user → 200, wrong role
on a role-gated route → 403, correct role → 200. The tests spin up their
own fake JWKS server and sign test tokens with a freshly generated ES256
key — no real Supabase project touched.

## 7. Manual exit check with a real token

To satisfy the roadmap's actual exit check ("a real logged-in token can
access a protected test route; a Teacher's token gets rejected on a
Principal-only route"), do this once against staging:

1. Log in through the frontend (Supabase phone/OTP) as a seeded Teacher
   account from your Stage 2 seed script.
2. Grab the `access_token` from Supabase's client-side session (browser
   devtools → Application → local storage, or log it temporarily).
3. Run:

```bash
curl -H "Authorization: Bearer <token>" https://<lightsail-ip>/api/test/protected
curl -H "Authorization: Bearer <token>" https://<lightsail-ip>/api/test/principal-only
```

Expected: first call → 200 with the teacher's user_id/role. Second call →
403. Repeat with a seeded Principal token and confirm both return 200.

## 8. A design note worth confirming with yourselves

`SupabaseAuth` currently rejects (403) any valid Supabase login with no
matching `users` row, rather than auto-creating one. That matches "Eng A
owns identity" from the roadmap — Principal-driven creation (Stage 4) is
what's supposed to provision accounts, not first-login. If you intended
self-service signup for any role (e.g. a parent creating their own account
before a Principal adds them), flag that now — it changes this middleware
from "look up" to "look up or create," and changes what Stage 4's
student/guardian creation flow needs to do (link vs. create).