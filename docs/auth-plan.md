# Authentication Plan

## Progress

| # | PR | Status | Notes |
|---|---|---|---|
| B1 | `feat/auth-users-db` | **Done** | Migration + sqlc queries + generated code |
| B2 | `feat/auth-core` | **In progress** | Auth service, middleware, OpenAPI, CORS wired — build passing. createuser CLI remaining |
| B3 | `feat/auth-createuser-cmd` | **Done** | `cmd/createuser/main.go` |
| F1 | `feat/auth-context` | Pending | |
| F2 | `feat/auth-api` | Pending | |

---

## Overview

Add JWT-based authentication between the sunbase frontend and winet-intergration backend so the API can be safely exposed over HTTPS.

No third-party auth provider. Everything is self-contained:
- Password hashing: bcrypt via existing `pkg/hasher` package
- Token signing: JWT (`github.com/golang-jwt/jwt/v5`)
- User creation: local CLI tool (not a public endpoint)
- Session persistence: refresh tokens stored in process memory (sync.Map)

---

## Token Strategy

| Token | Type | TTL | Storage (client) | Storage (server) |
|---|---|---|---|---|
| Access token | Signed JWT | 15 min | React memory / context | Stateless (validated by signature) |
| Refresh token | Opaque random string | 30 days | httpOnly, SameSite=Strict cookie | In-memory map (hashed key, lost on restart) |

**Flow:**
1. `POST /auth/login` — validates credentials, returns access JWT in body + sets refresh cookie
2. Frontend stores access token in React context (memory-only, not localStorage)
3. Every API request sends `Authorization: Bearer <access_token>`
4. On 401, frontend calls `POST /auth/refresh` (cookie is sent automatically) → gets new access token, retries original request
5. On page load, frontend calls `POST /auth/refresh` to restore session from cookie
6. `POST /auth/logout` — removes refresh token from in-memory store, clears cookie

**Why in-memory for refresh tokens:** No DB round-trip on refresh, no extra migration, trivially simple. The tradeoff is sessions are lost on server restart — users re-login, which is acceptable for a pet project. With few users the memory footprint is negligible.

**Why this split (access in memory, refresh in cookie):** Access token in memory prevents XSS token theft. Refresh token in httpOnly cookie prevents JS access while surviving page reloads.

---

## Backend PRs (winet-intergration)

### PR 1 — `feat/auth-users-db` ✓ Done

**Database migration** (users only — no token table needed)

`migrations/000005_create_users_table.up.sql`
```sql
CREATE TABLE IF NOT EXISTS users (
    id            SERIAL PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
```

`migrations/000005_create_users_table.down.sql`
```sql
DROP TABLE IF EXISTS users;
```

**sqlc queries** (`internal/pkg/database/queries/users.sql`)
```sql
-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = $1 LIMIT 1;

-- name: CreateUser :one
INSERT INTO users (username, password_hash) VALUES ($1, $2) RETURNING *;
```

Run `sqlc generate` to regenerate `internal/pkg/database/db/`.

Files created:
- `migrations/000005_create_users_table.up.sql`
- `migrations/000005_create_users_table.down.sql`
- `internal/pkg/database/queries/users.sql`
- `internal/pkg/database/db/users.sql.go` (generated)
- `internal/pkg/database/db/models.go` updated with `User` struct (generated)

---

### PR 2 — `feat/auth-core` (build passing, createuser CLI still pending as PR 3)

**New dependency**
```
github.com/golang-jwt/jwt/v5
```

**Config additions** (`internal/pkg/config/config.go`)
```go
type AuthConfig struct {
    JWTSecret       string        `env:"JWT_SECRET,required"`
    AccessTokenTTL  time.Duration `env:"JWT_ACCESS_TTL"  envDefault:"15m"`
    RefreshTokenTTL time.Duration `env:"JWT_REFRESH_TTL" envDefault:"720h"`
}
```
Add `AuthCfg AuthConfig` to `Config`.

**Auth service** (`internal/pkg/auth/auth.go`)

The service owns an in-memory store for refresh tokens:

```go
type tokenRecord struct {
    userID    int
    username  string
    expiresAt time.Time
}

type Service struct {
    secret          []byte
    accessTokenTTL  time.Duration
    refreshTokenTTL time.Duration
    tokens          sync.Map  // key: sha256(rawToken) hex string → tokenRecord
    db              UserStore
}
```

Responsibilities:
- `IssueAccessToken(userID int, username string) (string, error)` — signs JWT with HS256
- `ValidateAccessToken(tokenStr string) (*Claims, error)` — parses and validates JWT; no I/O
- `Login(ctx, username, password string) (accessToken, refreshToken string, error)` — looks up user via DB (one query, login only), verifies password with `pkg/hasher`, issues both tokens, stores hashed refresh token in `sync.Map`
- `Refresh(rawRefreshToken string) (newAccessToken string, error)` — SHA-256 hashes the raw token, looks up in `sync.Map`, checks expiry, issues new access token; no DB query
- `Logout(rawRefreshToken string)` — removes token from `sync.Map`; no DB query

The DB is only touched at login (one `GetUserByUsername` query). All refresh/logout operations are pure in-memory.

**In-memory store cleanup:** A background goroutine (started with the service) sweeps expired entries from the `sync.Map` periodically (e.g. hourly). With few users this is trivially cheap.

**OpenAPI additions** (`gen/api.yaml`)

Add to paths:
```yaml
/auth/login:
  post:
    requestBody:
      content:
        application/json:
          schema:
            $ref: "#/components/schemas/LoginRequest"
    responses:
      "200":
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/LoginResponse"
      "401":
        description: Invalid credentials

/auth/refresh:
  post:
    description: Uses httpOnly refresh token cookie
    responses:
      "200":
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/LoginResponse"
      "401":
        description: Refresh token invalid or expired

/auth/logout:
  post:
    responses:
      "204":
        description: Logged out
```

Add schemas:
```yaml
LoginRequest:
  type: object
  required: [username, password]
  properties:
    username:
      type: string
    password:
      type: string

LoginResponse:
  type: object
  required: [access_token]
  properties:
    access_token:
      type: string
```

Run `oapi-codegen` (via `make generate` or equivalent) to regenerate `pkg/server/api.gen.go`.

Files created/modified so far:
- `go.mod` / `go.sum` — added `github.com/golang-jwt/jwt/v5`
- `internal/pkg/config/config.go` — added `AuthConfig` struct and `AllowedOrigin` field to `Config`
- `internal/pkg/auth/auth.go` — new; `Service` with in-memory `sync.Map`, `Login`, `Refresh`, `Logout`, `ValidateAccessToken`, `StartCleanup`
- `internal/pkg/database/read.go` — added `GetUserByUsername` so `database.Database` satisfies `auth.UserStore`
- `gen/api.yaml` — added `/auth/login`, `/auth/refresh`, `/auth/logout` paths and `LoginRequest`/`LoginResponse` schemas
- `pkg/server/api.gen.go` — regenerated (includes `PostAuthLogin`, `PostAuthRefresh`, `PostAuthLogout`)
- `internal/pkg/server/server.go` — added `authSvc *auth.Service` field, updated `New()`, implemented `PostAuthLogin`, `PostAuthRefresh`, `PostAuthLogout`
- `internal/pkg/server/middleware.go` — replaced open-origin CORS with `allowedOrigin` param + credentials headers + OPTIONS preflight; added `AuthMiddleware`; added `ClaimsFromContext` helper
- `cmd/cmd.go` — construct `auth.Service`, start cleanup goroutine, pass authSvc + allowedOrigin to `startHTTPServer`, wire `AuthMiddleware` and updated `LoggingMiddleware` into middleware chain

**JWT auth middleware** (`internal/pkg/server/middleware.go`)

```go
func AuthMiddleware(authSvc AuthService) func(http.Handler) http.Handler
```

- Reads `Authorization: Bearer <token>` header
- Calls `authSvc.ValidateAccessToken()` — no I/O, just signature + expiry check
- On success: stores claims in request context
- On failure: returns 401
- Auth endpoints (`/auth/*`) and `/health` are exempt

**CORS update** (`internal/pkg/server/middleware.go`)

`LoggingMiddleware` currently reflects any Origin back without credentials support. When cookies are involved, the browser requires `Access-Control-Allow-Credentials: true` and a non-wildcard origin. Update to:
```go
// Only allow a configured ALLOWED_ORIGIN; reflect that back with credentials flag.
w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
w.Header().Set("Access-Control-Allow-Credentials", "true")
w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
```
Add `AllowedOrigin string \`env:"ALLOWED_ORIGIN,required"\`` to `Config`.

**Wire everything** (`cmd/cmd.go`)
- Construct `auth.Service` with DB + config, start its cleanup goroutine
- Add `AuthMiddleware` to the middleware chain
- Implement auth handlers in `internal/pkg/server/server.go`

**Refresh token cookie settings:**
```
Name:     "refresh_token"
HttpOnly: true
Secure:   true   (HTTPS only)
SameSite: Strict
Path:     /auth/refresh (scope cookie to refresh endpoint only)
MaxAge:   RefreshTokenTTL in seconds
```

---

### PR 3 — `feat/auth-createuser-cmd` ✓ Done

**New CLI tool:** `cmd/createuser/main.go`

Usage:
```
DATABASE_URL=postgres://... ./createuser --username alice --password s3cr3t
```

Logic:
1. Parse `--username` and `--password` flags
2. Validate: username non-empty, password >= 12 chars
3. Hash password using existing `pkg/hasher.HashPassword()`
4. Open DB connection using `pgxpool`
5. Run `CreateUser` sqlc query
6. Print success confirmation

This tool is run locally (or via SSH) and never exposed publicly.

---

## Frontend PRs (sunbase)

### PR 1 — `feat/auth-context`

**`src/lib/auth.ts`** — raw API calls for auth (no auth header needed):
```ts
export async function login(username: string, password: string): Promise<{ access_token: string }>
export async function refresh(): Promise<{ access_token: string }>
export async function logout(): Promise<void>
```

**`src/context/AuthContext.tsx`** — React context:
```ts
interface AuthContextValue {
  accessToken: string | null;
  login(username: string, password: string): Promise<void>;
  logout(): Promise<void>;
  isRestoring: boolean; // true while attempting refresh on page load
}
```

On mount: calls `refresh()` to restore session from cookie. Sets `isRestoring` during this.

**`src/pages/Login.tsx`** — login form using existing shadcn/ui `Card`, `Button` components. Shows error on bad credentials. Redirects to `/` on success.

**`src/components/ProtectedRoute.tsx`**
```tsx
// Renders children if authenticated, redirects to /login if not.
// Shows nothing (or a spinner) while isRestoring is true.
```

**`src/App.tsx`** updates:
- Wrap app in `AuthProvider`
- Add `/login` route
- Wrap `/` with `ProtectedRoute`

---

### PR 2 — `feat/auth-api`

**`src/lib/api.ts`** updates:
- Accept access token parameter (sourced from AuthContext)
- Send `Authorization: Bearer <token>` on all requests
- On 401 response: call `refresh()` once, update stored token, retry original request
- On second 401 (refresh failed or server restarted): redirect to `/login`

The cleanest pattern: export a `createApiClient(getToken, onUnauthorized)` factory called from AuthContext, so hooks use a pre-configured client rather than importing raw fetch calls directly.

---

## Environment Variables Summary

### winet-intergration
| Variable | Required | Default | Notes |
|---|---|---|---|
| `JWT_SECRET` | Yes | — | Min 32 chars, random. Generate with `openssl rand -base64 32` |
| `JWT_ACCESS_TTL` | No | `15m` | Go duration string |
| `JWT_REFRESH_TTL` | No | `720h` | Go duration string (30 days) |
| `ALLOWED_ORIGIN` | Yes | — | Frontend origin e.g. `https://sunbase.example.com` |

### sunbase
No new env vars needed. Auth endpoints are relative to existing `VITE_API_BASE_URL`.

---

## PR Order / Dependencies

```
Backend:
  PR 1 (auth-users-db)
    └── PR 2 (auth-core)          ← depends on DB schema + sqlc generated code
          └── PR 3 (createuser)   ← depends on sqlc generated code

Frontend:
  PR 1 (auth-context)             ← can start once backend PR 2 endpoints are defined
    └── PR 2 (auth-api)           ← depends on AuthContext
```

Backend PR 1 and Frontend PR 1 can be drafted in parallel since the frontend PR only needs the API contract (from the OpenAPI spec), not the running backend.

---

## Security Notes

- JWT signed with HS256 using a secret loaded from env (`JWT_SECRET`). Rotating `JWT_SECRET` immediately invalidates all access tokens; users will re-login on next refresh attempt.
- Refresh tokens stored as SHA-256(rawToken) in the in-memory map — raw token only travels over the wire, never stored plaintext. A server restart clears the store; users re-login.
- Refresh cookie is `Secure` (HTTPS-only), `HttpOnly` (no JS access), `SameSite=Strict` (CSRF-safe).
- bcrypt cost factor 10 (already set in `pkg/hasher`).
- No user registration endpoint — users created only via local CLI or direct DB access.
- `ALLOWED_ORIGIN` must be set to the exact frontend origin; wildcard is not permitted when `credentials: include` is used.
