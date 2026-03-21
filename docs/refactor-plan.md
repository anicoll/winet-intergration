# Refactor Plan: winet-integration

This document outlines a phased refactor of the winet-integration service. The goal is to
fix critical bugs, improve composability, adopt modern tooling, and make the codebase
testable and maintainable.

---

## Current Architecture Problems

Before the steps, here is a summary of the bugs and structural issues driving this plan.

### Critical Bugs

#### 1. `timeoutErrChan` nil-channel panic

**File:** [internal/pkg/winet/winet.go](../internal/pkg/winet/winet.go)

`SubscribeToTimeout()` initialises `s.timeoutErrChan` and returns it. However, in
[cmd/cmd.go](../cmd/cmd.go), `Connect()` is called first, and `SubscribeToTimeout()` is
called afterward. During the `Connect()` flow, if the server sends `"login timeout"`, the
handler tries to send to `s.timeoutErrChan` — which is `nil`. Sending to a nil channel
blocks forever; the goroutine hangs silently.

**Fix:** Initialise `timeoutErrChan` in `New()`, not lazily in `SubscribeToTimeout()`.

---

#### 2. Reconnect does not close the old connection

**File:** [internal/pkg/winet/winet.go:138](../internal/pkg/winet/winet.go#L138)

`reconnect()` creates a brand-new `ws.New()` struct and assigns it to `s.conn`, but the
old connection's goroutines (`readLoop`, `setupPing`) are still running. They hold a
closure over the old `*Conn`, and will continue calling `s.onMessage` and `s.onError`
on the live `*service`. This creates a race condition where two goroutines are
simultaneously processing messages and mutating shared state (`s.storedData`,
`s.currentDevice`, `s.token`).

**Fix:** Call `s.conn.Close()` before assigning a new connection.

---

#### 3. `processed` channel deadlock on disconnect

**Files:** [internal/pkg/winet/devicelist.go](../internal/pkg/winet/devicelist.go),
[internal/pkg/winet/real.go](../internal/pkg/winet/real.go),
[internal/pkg/winet/inverter_commands.go](../internal/pkg/winet/inverter_commands.go)

`handleDeviceListMessage` calls `s.waiter()` which blocks on `<-s.processed`. The signal
is sent from `handleRealMessage` or `handleDirectMessage`. If the WebSocket connection
drops between sending a query and receiving its response, the signal is never sent and
`waiter()` blocks forever.

Consequences:

- The device list goroutine hangs indefinitely.
- Any HTTP request that calls an inverter command (`SendChargeCommand`, etc.) also calls
  `s.waiter()` and deadlocks.
- On reconnect, a new `handleDeviceListMessage` goroutine starts, but the old one is
  still blocked. Now two goroutines race to send `sendDeviceListRequest`.

**Fix:** Replace the bare channel block with a `select` that also handles `ctx.Done()` and
a timeout. Use a proper request/response correlation map (`map[requestID]chan<- response`)
so commands can be matched to their responses individually.

---

#### 4. Multiple concurrent `handleDeviceListMessage` goroutines

**File:** [internal/pkg/winet/winet.go:117](../internal/pkg/winet/winet.go#L117)

`onMessage` dispatches `handleDeviceListMessage` with `go`. Each invocation runs a full
polling loop: send queries, wait, sleep, send another device list request, recurse. On
reconnect this accumulates goroutines. Each goroutine independently polls and sends
requests, leading to duplicate data and message ordering violations.

**Fix:** The polling loop belongs in a dedicated, single goroutine owned by `Connect()`,
not inside the message handler.

---

#### 5. `ticker` not stopped in `handleDeviceListMessage`

**File:** [internal/pkg/winet/devicelist.go:53](../internal/pkg/winet/devicelist.go#L53)

```go
ticker := time.NewTicker(time.Second * s.cfg.PollInterval)
<-ticker.C
s.sendDeviceListRequest(c)
```

`ticker.Stop()` is never called. Minor goroutine/timer leak that compounds with bug #4.

---

#### 6. `sendIfErr` does not stop execution

**File:** [internal/pkg/winet/winet.go:45](../internal/pkg/winet/winet.go#L45)

`sendIfErr` sends an error to the channel and returns. The caller continues executing.
For example in `onconnect`, after a marshal error the code proceeds to call `c.Send` with
garbage data. In `handleDirectMessage`, after a float parse error, computation continues
with zero values. All of these should `return` after the error.

---

#### 7. `http.DefaultTransport` mutated globally

**File:** [internal/pkg/winet/properties.go:19](../internal/pkg/winet/properties.go#L19)

```go
http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
```

This mutates the global default HTTP transport for the entire process. Any subsequent
outbound HTTP request (e.g., to the Amber API) inherits the insecure TLS config.

**Fix:** Use a dedicated `http.Client` with its own transport for the properties fetch.

---

#### 8. `currentDevice` race condition

**Files:** [internal/pkg/winet/devicelist.go](../internal/pkg/winet/devicelist.go),
[internal/pkg/winet/real.go](../internal/pkg/winet/real.go),
[internal/pkg/winet/direct.go](../internal/pkg/winet/direct.go)

`handleDeviceListMessage` writes `s.currentDevice` while `handleRealMessage` and
`handleDirectMessage` (both called with `go`) read it. No mutex protects this field.

---

#### 9. `handleDirectMessage` never publishes data

**File:** [internal/pkg/winet/direct.go:83](../internal/pkg/winet/direct.go#L83)

The function builds `datapointsToPublish` but never calls `publisher.PublishData`. It
also uses `data.CurrentUnit` as the unit for the computed power field instead of `"W"`.

---

#### 10. Global publisher registry has no mutex

**File:** [internal/pkg/publisher/publisher.go:19](../internal/pkg/publisher/publisher.go#L19)

`registerdPublishers` is a plain `map[string]publisher`. `RegisterPublisher` writes to it
and `PublishData` / `RegisterDevice` iterate over it without any synchronisation.

---

#### 11. `GetLatestProperties` SQL is incorrect

**File:** [internal/pkg/database/read.go:68](../internal/pkg/database/read.go#L68)

```sql
GROUP BY id, identifier, slug
```

Grouping by `id` (the primary key) makes the GROUP BY a no-op — every row is unique.
The actual deduplication to "latest per identifier+slug" happens entirely in Go with a
`map`, making the SQL aggregation pointless and fetching far more data than necessary.

**Fix:** Use a window function or `DISTINCT ON (identifier, slug) ORDER BY slug, time_stamp DESC`.

---

#### 12. `MqttCfg` reuses the wrong config type

**File:** [internal/pkg/config/config.go](../internal/pkg/config/config.go)

`MqttCfg *WinetConfig` reuses the Winet config struct, which contains `Ssl bool` and
`PollInterval time.Duration` — neither of which applies to MQTT.

---

#### 13. Amber configuration not in the config struct

**File:** [cmd/cmd.go:256](../cmd/cmd.go#L256)

`AMBER_HOST` and `AMBER_TOKEN` are read from environment variables inside
`startAmberPriceService`. They are invisible in the config struct and cannot be validated
at startup or passed in tests.

---

#### 14. `contxt` package creates detached contexts

**File:** [internal/pkg/contxt/contxt.go](../internal/pkg/contxt/contxt.go)

`contxt.NewContext(time.Second*5)` creates a context with a 5-second timeout derived from
`context.Background()`, not from the parent. If the parent context is cancelled (shutdown),
in-flight publish operations still run for up to 5 seconds.

---

#### 15. Mixed database access patterns

The codebase mixes three approaches:

- Raw SQL strings with `database/sql` in `write.go` and `read.go`
- Generated `dbtpl` models called as methods (`prop.Insert(ctx, db)`) in `write.go`
- Both `jackc/pgx/v5` and `github.com/lib/pq` are imported as drivers

This makes it hard to reason about queries, prevents type-safe parameter binding, and the
`dbtpl` generator is non-standard with very little community support.

---

#### 16. Commented-out services with no clear path forward

`startDbCleanupService` and `startDecisionService` are fully commented out in `cmd.go`.
`dailyFeedinEnabler` is hardcoded to 5 PM Adelaide time with no configuration.

---

## Refactor Steps

---

### Step 1 — Fix critical websocket bugs (targeted, surgical) ✅ DONE

**Goal:** Stop crashes and deadlocks without restructuring the code yet.

Changes:

- Initialise `timeoutErrChan` with buffer 1 in `New()`, remove lazy init from `SubscribeToTimeout()`.
- In `reconnect()`, call `s.conn.Close()` before creating a new connection.
- Add `defer ticker.Stop()` in `handleDeviceListMessage`.
- Add `ctx.Done()` arm and a configurable timeout to `s.waiter()`.
- Add a mutex to guard `s.currentDevice` reads/writes.
- Fix `sendIfErr` callers to `return` after calling it (or convert handlers to return errors).
- Fix `handleDirectMessage` to call `publisher.PublishData` and use `"W"` for the power unit.
- Add `sync.RWMutex` to the global publisher registry.

**Bugs fixed:** #1, #2, #3, #5, #6, #8, #9, #10
**Bugs deferred to later steps:** #4 (goroutine accumulation — Step 3), #7 (DefaultTransport — Step 2),
#11–#16 (config/SQL — Steps 2–4)

**Testing:** Covered by Step 10 below.

---

### Step 2 — Fix `http.DefaultTransport` mutation and config types ✅ DONE

**Goal:** Eliminate global side effects and mistyped config.

- In `properties.go`, replaced `http.DefaultTransport` mutation with a package-scoped
  `propertiesClient` (`*http.Client`) that has its own `*http.Transport` with
  `InsecureSkipVerify` scoped only to the inverter properties fetch.
- Added a dedicated `MQTTConfig` struct:
  ```go
  type MQTTConfig struct {
      Host     string
      Username string
      Password string
  }
  ```
- Added `AmberConfig` to the config struct:
  ```go
  type AmberConfig struct {
      Host  string
      Token string
  }
  ```
- Added `Timezone string` field to `Config` (default `"Australia/Adelaide"` via CLI flag).
- Added `Config.Validate()` method in the config package — single place for all required-field
  checks. Removed the now-redundant `validateConfig` function from `cmd.go`.
- `startAmberPriceService` now accepts `*config.AmberConfig` instead of reading
  `AMBER_HOST`/`AMBER_TOKEN` env vars directly; config is visible and validated at startup.
- `dailyFeedinEnabler` now takes a `timezone string` parameter instead of the hardcoded
  `"Australia/Adelaide"` string.
- Added `--amber-host`, `--amber-token`, and `--timezone` CLI flags to `main.go`.

**Testing:** Config validation unit tests added in `internal/pkg/config/config_test.go`.

---

### Step 3 — Restructure the winet service (major) ✅ DONE

**Goal:** Replace the monolithic `service` struct with composable, testable pieces.

Key design changes made:

- **Eliminated `processed chan any` and `timeoutErrChan chan error`.** Replaced with:
  - `pendingCmd` — a thread-safe single-slot struct for serial request/response
    correlation. `wait(ctx)` registers a buffered channel; `deliver(v)` sends to it
    non-blockingly. No goroutine leak if nobody is waiting (no-op deliver).
  - `events chan SessionEvent` — session lifecycle events (timeout, errors) delivered
    here instead of a separate channel. Exposed via `Events() <-chan SessionEvent`.
- **Polling loop moved to a dedicated goroutine** (`runPollLoop` in `poller.go`).
  Cancelling the poll context (stored as `cancelPoll`) is the only way to stop it.
  `Connect()` cancels the previous poll loop before starting a new one — no goroutine
  accumulation on reconnect (fixes Bug #4).
- **`loginReady chan struct{}`** created fresh on every `Connect()` call. Closed by
  `handleLoginMessage` to unblock `runPollLoop` after the login handshake completes.
  The poll loop sends the first device list request — not the login handler.
- **`handleDeviceListMessage` simplified** to parse + `pending.deliver(list)`. All
  device iteration, per-stage querying, ticker, and recursion removed (fixes Bug #5 —
  ticker leak; fixes Bug #4 — goroutine accumulation).
- **`handleRealMessage` / `handleDirectMessage`** now call `pending.deliver` instead of
  `processed <-`.
- **`handleParamMessage` / inverter commands** use `pending.wait(s.ctx)` / `pending.deliver`.
- **`SubscribeToTimeout()` removed.** `cmd.go` now selects on `winetSvc.Events()` and
  checks `event.Err` against `winet.ErrTimeout`.

New files:

- `internal/pkg/winet/poller.go` — `runPollLoop`, `queryDevices`, `sendQueryRequest`

Sub-package split (`session/`, `protocol/`, `poller/`, `commands/`) deferred — the
behavioural fixes are in place; the directory reorganisation can happen as a follow-up
refactor without further functional change.

---

### Step 4 — Replace `dbtpl` with `sqlc`

**Goal:** Type-safe, maintainable SQL with a widely-supported code generator.

Tooling:

- Use [sqlc](https://sqlc.dev) to generate Go code from SQL queries.
- Use `pgx/v5` directly (drop `lib/pq` entirely).
- Drop the `database/sql` wrapper; use `pgxpool.Pool` directly.

Steps:

1. Add a `sqlc.yaml` config at the project root pointing at the migrations folder.
2. Write SQL query files in `internal/pkg/database/queries/`:
   - `properties.sql` — insert, latest-per-device (fixed with `DISTINCT ON`), range query
   - `devices.sql` — upsert
   - `amber_prices.sql` — upsert, range query
3. Run `sqlc generate` to produce typed structs and query functions in
   `internal/pkg/database/sqlc/`.
4. Delete the `internal/pkg/models/` directory and all `*.dbtpl.go` files.
5. Update all callers to use the generated sqlc functions.
6. Fix the `GetLatestProperties` query:
   ```sql
   -- name: GetLatestProperties :many
   SELECT DISTINCT ON (identifier, slug)
       id, time_stamp, unit_of_measurement, value, identifier, slug
   FROM property
   WHERE time_stamp > NOW() - INTERVAL '1 day'
   ORDER BY identifier, slug, time_stamp DESC;
   ```
7. Remove `github.com/lib/pq` from `go.mod`.

**Testing:** Add integration tests using `testcontainers-go` to spin up a real Postgres
instance and verify all query functions against actual schema migrations.

---

### Step 5 — Replace global publisher with dependency injection ✅ DONE

**Goal:** Remove the global registry; make the publishing pipeline testable.

Changes made:

- **`DataPoint` struct** — typed replacement for `map[string]any` passed between
  normalizer → backend publishers.
- **`Publisher` interface** (backends: MQTT, database):
  ```go
  type Publisher interface {
      Write(ctx context.Context, data []DataPoint) error
      RegisterDevice(ctx context.Context, device *model.Device) error
  }
  ```
- **`DataPublisher` interface** (injected into the winet service):
  ```go
  type DataPublisher interface {
      PublishData(ctx context.Context, devices map[model.Device][]model.DeviceStatus) error
      RegisterDevice(ctx context.Context, device *model.Device) error
  }
  ```
- **`Normalizer` type** (`publisher/normalizer.go`) — converts a raw `DeviceStatus` into
  a `DataPoint`: applies unit conversions (kWp→kW, ℃→°C, kvar→var×1000, kVA→VA×1000),
  nil/`"--"` → `"0.00"` for numeric sensors, filters ignored slugs.
- **`MultiPublisher`** — implements `DataPublisher`; normalizes, deduplicates (instance-
  scoped `sync.Map` keyed by `identifier_slug`), then fans out `[]DataPoint` to all
  registered `Publisher` backends. Backend errors are logged but do not fail the call.
- All package-level globals (`registerdPublishers`, `sensors`, `publishersMu`) and global
  functions (`RegisterPublisher`, `PublishData`, `RegisterDevice`) removed.
- `winet.New()` updated to accept `publisher.DataPublisher` as a required parameter.
- `poller.go`, `real.go`, `direct.go` now call `s.publisher.RegisterDevice` /
  `s.publisher.PublishData` instead of package-level functions.
- `mqtt/write.go` and `database/write.go` updated: `Write` takes `[]publisher.DataPoint`.
- `cmd.go` wires everything: `publisher.NewMultiPublisher(db, mqttPublisher)` is created
  and passed to `winet.New()`.
- `.mockery.yaml` — added `DataPublisher` interface; `make generate-mocks` now deletes
  all `mocks/**/*.go` before regenerating to prevent stale mocks from accumulating.

**Testing:**

- `publisher/publisher_test.go` — table-driven `Normalizer` tests (all unit conversions,
  ignored slugs, nil/dash values, text sensors, dot-stripping in identifier); `MultiPublisher`
  tests for fan-out, dedup, error isolation, `RegisterDevice` fan-out. Uses a local
  `stubPublisher` to avoid a mock import cycle.
- `winet/winet_test.go` — three new tests verify that `queryDevices` calls
  `RegisterDevice`, and that `handleRealMessage` / `handleDirectMessage` each call
  `PublishData`, using the generated `DataPublisher` mock.

---

### Step 6 — Replace gorilla/mux with `net/http` (Go 1.22+) ✅ DONE

**Goal:** Reduce dependencies; use stdlib routing.

Go 1.22 added method and path parameter support to `net/http.ServeMux`. Since the project
already targets Go 1.26, gorilla/mux adds no value.

Changes made:

- `gen/config.yaml` — switched `gorilla-server: true` to `std-http-server: true`.
- Regenerated `pkg/server/api.gen.go` with oapi-codegen v2.4.1 (v2.6.0 has a build bug
  with the `kin-openapi` type change; pinned via `go run ...@v2.4.1`). Generated code now
  uses `r.PathValue("param")` for path parameters and `StdHTTPServerOptions` / `http.NewServeMux()`.
- `cmd/cmd.go` — removed `gorilla/mux` import and `mux.CORSMethodMiddleware`; replaced
  `GorillaServerOptions{BaseRouter: r, ...}` with `StdHTTPServerOptions{...}`.
- `server/server.go` — fixed all three bugs:
  - `GetPropertyIdentifierSlug`: `Content-Type` header now set before `json.NewEncoder(w).Encode(...)`.
  - `handleError`: introduced `clientError` sentinel type; errors wrapping it return 400,
    all others return 500.
  - `PostBatteryState`, `PostInverterFeedin`, `PostInverterState`: write `204 No Content`
    on success instead of the string `"success"`.
  - Removed `OptionsBatteryState` (no longer in the generated `ServerInterface`).
  - `unmarshalPayload` now wraps read/decode errors in `clientError` so JSON parse failures
    return 400 instead of 500.
- `go.mod` — `github.com/gorilla/mux` removed via `go mod tidy`.
- `server_test.go` — updated success assertions from `200` → `204`; missing-power error
  assertion updated from `500` → `400`.

---

### Step 7 — Configuration: env vars + YAML with validation ✅ DONE

**Goal:** Single, validated, fully-documented config source.

Changes made:

- **`internal/pkg/config/config.go`** — added `env` struct tags from
  [github.com/caarlos0/env/v11](https://github.com/caarlos0/env) to all config types.
  Sub-config fields changed from pointers (`*WinetConfig`) to values (`WinetConfig`) so
  caarlos0/env can parse nested structs directly. Added `Load() (*Config, error)` as the
  single entry point for configuration; removed the hand-written `Validate()` method
  (validation is now handled by `env:"...,required"` tags and defaults by `envDefault:`).
  ```go
  type Config struct {
      WinetCfg         WinetConfig
      MqttCfg          MQTTConfig
      AmberCfg         AmberConfig
      LogLevel         string `env:"LOG_LEVEL"          envDefault:"info"`
      DBDSN            string `env:"DATABASE_URL,required"`
      MigrationsFolder string `env:"MIGRATIONS_FOLDER"  envDefault:"migrations"`
      Timezone         string `env:"TIMEZONE"           envDefault:"Australia/Adelaide"`
  }
  type WinetConfig struct {
      Host         string        `env:"WINET_HOST,required"`
      Username     string        `env:"WINET_USERNAME,required"`
      Password     string        `env:"WINET_PASSWORD,required"`
      Ssl          bool          `env:"WINET_SSL"`
      PollInterval time.Duration `env:"WINET_POLL_INTERVAL" envDefault:"30s"`
  }
  ```
- **`cmd/cmd.go`** — removed `WinetCommand` (the urfave/cli entry point) and the
  `urfave/cli/v2` import. Renamed `run` → `Run` (exported). Updated two pointer call
  sites: `winet.New(&cfg.WinetCfg, ...)` and `startAmberPriceService(ctx, &cfg.AmberCfg, ...)`.
- **`main.go`** — replaced the entire `cli.App` setup (95 lines) with a 7-line main:
  parse env via `config.Load()`, then call `cmd.Run()`.
- **`go.mod`** — added `github.com/caarlos0/env/v11 v11.4.0`; removed
  `github.com/urfave/cli/v2` and `github.com/goburrow/modbus` (dead `md()` function
  removed from main.go); upgraded `github.com/oapi-codegen/runtime v1.1.2 → v1.3.0`
  (required for v2.6.0-generated code to compile).

**Testing:** `internal/pkg/config/config_test.go` rewritten to test `Load()` via
environment variables using `t.Setenv` / `os.Unsetenv`: success case, defaults, all four
required-field missing cases, and a custom-values case. Coverage: 100%.

---

### Step 8 — Reconnect and session management (follow-up to Step 3)

**Goal:** Robust, observable reconnect behaviour.

The current retry loop in `startWinetService` has a fixed 5-second sleep with no
backoff. On repeated failures (e.g., inverter rebooting) this hammers the device.

Changes:

- Implement exponential backoff with jitter using the standard pattern:
  ```go
  backoff := min(baseDelay * (1 << attempt), maxDelay)
  sleep(backoff + jitter())
  ```
- Expose a `HealthStatus` (connected/disconnected/reconnecting) via the HTTP server
  at `GET /health`.
- Log each reconnect attempt with the backoff duration and attempt count.
- After `maxAttempts` consecutive failures, surface a fatal error to the errgroup so
  the process exits with a non-zero code and can be restarted by the container runtime.

---

### Step 9 — Re-enable and complete the commented-out services

**Goal:** Restore `startDbCleanupService` and `startDecisionService` to working order.

`startDbCleanupService`:

- Was commented out with no explanation. Re-enable it; it is a simple cron job.
- Make the cleanup schedule configurable via env var.

`startDecisionService` / `logic.NextBestAction`:

- Logic is incomplete: the charge/feedin decision is made but there is no hysteresis
  (if price fluctuates around zero, the inverter is toggled every 5 seconds).
- Add a minimum dwell time before switching modes (e.g., 5 minutes).
- Add logging for every decision with the current price and action taken.
- Make the poll interval configurable; remove the hardcoded `time.Sleep(5 * time.Second)`.

`dailyFeedinEnabler`:

- Move the 5 PM schedule into the config struct as a cron expression.
- Make the timezone derive from `Config.Timezone` instead of a hardcoded string.

---

### Step 10 — Testing infrastructure

**Goal:** Meaningful test coverage at all layers.

| Layer                     | Tool                                | What to test                                                                                                            | Status                      |
| ------------------------- | ----------------------------------- | ----------------------------------------------------------------------------------------------------------------------- | --------------------------- |
| Websocket (`pkg/sockets`) | existing `httptest` approach        | expand: close-during-read, ping-stop-on-close, callback ordering                                                        | pending                     |
| Winet protocol            | mockery + `winet_test.go`           | login flow, timeout regression, reconnect close, send errors, pendingCmd, poll loop, queryDevices, all message handlers | ✅ done                     |
| DB queries                | `testcontainers-go` + real Postgres | all sqlc queries, migration correctness, DISTINCT ON dedup                                                              | pending (blocked on Step 4) |
| Publisher                 | mockery + `publisher_test.go`       | unit conversion, slug-ignore, dedup, nil/dash values, text sensors, fan-out                                             | ✅ done                     |
| HTTP server               | mockery + `server_test.go`          | all battery/inverter/feedin states, missing-power error, GetProperties JSON + DB error                                  | ✅ done                     |
| Logic                     | mockery + `logic_test.go`           | `NextBestAction` decision table: negative/zero/positive price, feedin, forecast exclusion, DB error                     | ✅ done                     |

**Completed in this PR:**

- Exported all previously-unexported interfaces (`publisher.Publisher`, `logic.WinetCommands`,
  `logic.Database`, `server.Database`) to make them mockable.
- Added `.mockery.yaml` config and `make generate-mocks` Makefile target (runs mockery via
  Docker: `vektra/mockery:v3`).
- Generated mocks under `mocks/` for all four interface groups.
- Replaced all hand-written mock structs across every test file with the generated mocks,
  using the EXPECT() fluent API and `AssertExpectations` via `t.Cleanup`.

**Still to do:**

- Add a `make test` target that runs unit tests and a separate `make test-integration`
  target that requires Docker (for `testcontainers-go`).
- Expand `pkg/sockets` tests (close-during-read, ping lifecycle).
- DB integration tests — blocked on Step 4 (sqlc migration).

---

## Dependency Changes Summary

| Action | Package                                                                |
| ------ | ---------------------------------------------------------------------- |
| Add    | `github.com/caarlos0/env/v11`                                          |
| Add    | `github.com/sqlc-dev/sqlc` (codegen tool only, `tools.go`)             |
| Add    | `github.com/testcontainers/testcontainers-go` (test only)              |
| Remove | `github.com/gorilla/mux`                                               |
| Remove | `github.com/lib/pq`                                                    |
| Remove | `github.com/urfave/cli/v2`                                             |
| Remove | `github.com/gosimple/slug` (replace with inline `strings.NewReplacer`) |
| Keep   | `github.com/gorilla/websocket`                                         |
| Keep   | `github.com/eclipse/paho.mqtt.golang`                                  |
| Keep   | `github.com/golang-migrate/migrate/v4`                                 |
| Keep   | `github.com/jackc/pgx/v5`                                              |
| Keep   | `github.com/oapi-codegen/oapi-codegen/v2`                              |
| Keep   | `github.com/robfig/cron/v3`                                            |
| Keep   | `go.uber.org/zap`                                                      |
| Keep   | `golang.org/x/sync`                                                    |

---

## Execution Order

Steps are ordered to avoid blocking on each other. Steps 1 and 2 are safe to do on
`main` immediately (bug fixes). Steps 3–7 are best done on a feature branch together
since they touch the same files. Steps 8–10 build on top.

```
Step 1  Fix websocket bugs          ✅ merged to main
Step 10 Testing (unit, mockery)     ✅ merged to main (partial — unit tests done)
Step 2  Fix transport + config types ✅ merged to main
Step 3  Restructure winet service   ✅ merged to main
Step 4  sqlc migration               ─┐  ✅ done (feature/refactor branch)
Step 5  DI publisher                  ├─ ✅ done (feature/refactor branch)
Step 6  stdlib HTTP                   ├─ ✅ done (feature/refactor branch)
Step 7  Config/CLI removal           ─┘  ✅ done (feature/refactor branch) → merge to main
Step 8  Reconnect backoff           → main
Step 9  Re-enable services          → main
Step 10 Testing (integration, DB)   → ongoing, parallel with Steps 3–9
```
