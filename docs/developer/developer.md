# Developer Guide

This document covers the architecture, package structure, configuration, data flow, and development workflows for the `winet-integration` service.

## Table of contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Package structure](#package-structure)
- [Configuration reference](#configuration-reference)
- [Data flow](#data-flow)
- [Services started at runtime](#services-started-at-runtime)
- [Database](#database)
- [Code generation](#code-generation)
- [HTTP API](#http-api)
- [Authentication](#authentication)
- [Feed-in automation](#feed-in-automation)
- [MQTT publishing](#mqtt-publishing)
- [Testing](#testing)
- [Local development](#local-development)

---

## Overview

`winet-integration` is a single Go binary that:

1. Maintains a persistent WebSocket connection to a Sungrow WiNet-S dongle
2. Polls real-time inverter/battery data and writes it to PostgreSQL and MQTT
3. Polls the Amber Electric API for live electricity prices and daily usage
4. Automatically enables grid feed-in export when prices are favourable
5. Exposes a JWT-authenticated REST API to query data and send inverter commands

---

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                        cmd.Run()                             │
│                                                              │
│  ┌─────────────┐  ┌──────────────┐  ┌────────────────────┐  │
│  │  WiNet svc  │  │  Amber price │  │  Amber usage svc   │  │
│  │  (WebSocket)│  │  svc (cron)  │  │  (cron, daily)     │  │
│  └──────┬──────┘  └──────┬───────┘  └────────────────────┘  │
│         │                │                                   │
│         ▼                ▼                                   │
│  ┌─────────────────────────────┐   ┌──────────────────────┐  │
│  │     MultiPublisher          │   │  Feed-in Controller   │  │
│  │  (Database + MQTT backends) │◄──│  (Evaluate on price   │  │
│  └─────────────────────────────┘   │   update)             │  │
│                                    └──────────────────────┘  │
│  ┌─────────────────────────────┐                             │
│  │     HTTP Server (:8000)     │                             │
│  │  (REST API + health check)  │                             │
│  └─────────────────────────────┘                             │
└──────────────────────────────────────────────────────────────┘
         │                │
         ▼                ▼
    PostgreSQL          MQTT broker
```

All services run in the same process as goroutines coordinated by `golang.org/x/sync/errgroup`. A shared error channel carries non-fatal errors from cron jobs; a fatal error from any goroutine shuts the whole process down via `errgroup`.

---

## Package structure

```
.
├── main.go                        Entry point — loads config, calls cmd.Run
├── cmd/
│   ├── cmd.go                     Orchestration: wires services together, starts goroutines
│   └── createuser/main.go         CLI tool to add a user to the database
├── internal/pkg/
│   ├── amber/                     Amber Electric API client wrapper
│   ├── auth/                      JWT auth service (issue, refresh, revoke)
│   ├── config/                    Environment variable config loading (caarlos0/env)
│   ├── contxt/                    Context helpers
│   ├── database/
│   │   ├── db/                    sqlc-generated query code (DO NOT edit manually)
│   │   ├── migration/             golang-migrate runner
│   │   ├── queries/               Raw SQL query files (sqlc source)
│   │   ├── postgres.go            Database struct implementing read/write interfaces
│   │   ├── read.go                Query methods (GetProperties, GetAmberPrices, …)
│   │   └── write.go               Write methods (WriteProperties, WriteAmberPrices, …)
│   ├── feedin/                    Automatic feed-in export controller
│   ├── logic/                     (Legacy — decision logic, currently disabled)
│   ├── model/                     Shared domain types (Device, DeviceStatus, MQTT models)
│   ├── mqtt/                      MQTT publisher backend
│   ├── publisher/                 MultiPublisher — normalises and fans out data
│   └── server/                    HTTP handler implementations + middleware
├── pkg/
│   ├── amber/                     oapi-codegen generated Amber API client
│   ├── hasher/                    bcrypt password hashing helper
│   ├── server/                    oapi-codegen generated server interfaces/types
│   └── sockets/                   WebSocket connection abstraction
├── gen/
│   ├── api.yaml                   OpenAPI spec for the winet-integration REST API
│   ├── config.yaml                oapi-codegen config for server generation
│   └── amber/
│       ├── api.json               Amber Electric OpenAPI spec
│       └── config.yaml            oapi-codegen config for Amber client generation
├── mocks/                         mockery-generated test mocks
├── mosquitto/                     Mosquitto MQTT broker config for Docker
├── docker-compose.yml             PostgreSQL + MQTT for local dev
└── grafana-docker-compose.yml     Optional Grafana instance
```

---

## Configuration reference

All config is loaded from environment variables at startup (no config file at runtime). The struct is defined in [internal/pkg/config/config.go](../internal/pkg/config/config.go).

### Required

| Variable | Description |
|---|---|
| `DATABASE_URL` | PostgreSQL DSN, e.g. `postgres://postgres:postgres@localhost:5432/winet` |
| `WINET_HOST` | WiNet-S IP or hostname |
| `WINET_USERNAME` | WiNet-S login username |
| `WINET_PASSWORD` | WiNet-S login password |
| `JWT_SECRET` | Secret used to sign JWTs |
| `ALLOWED_ORIGIN` | Comma-separated list of allowed CORS origins |
| `AMBER_HOST` | Amber API base URL |
| `AMBER_TOKEN` | Amber API bearer token |

### Optional

| Variable | Default | Description |
|---|---|---|
| `WINET_SSL` | `false` | Use `wss://` (port 443) instead of `ws://` (port 8082) |
| `WINET_POLL_INTERVAL` | `30s` | How often to poll device data |
| `MQTT_HOST` | — | MQTT broker address |
| `MQTT_USERNAME` | — | MQTT username |
| `MQTT_PASSWORD` | — | MQTT password |
| `JWT_ACCESS_TTL` | `15m` | Access token lifetime |
| `JWT_REFRESH_TTL` | `720h` | Refresh token lifetime (30 days) |
| `SECURE_COOKIES` | `true` | Set `Secure` flag on refresh token cookie |
| `LOG_LEVEL` | `info` | Zap log level (`debug`, `info`, `warn`, `error`) |
| `MIGRATIONS_FOLDER` | `migrations` | Path to SQL migration files |
| `TIMEZONE` | `Australia/Adelaide` | Timezone for cron schedules and time-of-day logic |
| `AMBER_SITES` | — | Comma-separated Amber site IDs; if set, skips the sites API call |

---

## Data flow

### Inverter data

```
WiNet-S device
  └─ WebSocket (ws://<host>:8082/ws/home/overview)
       └─ winet.service.onMessage()
            ├─ handleConnectMessage  → sends login request
            ├─ handleLoginMessage    → starts poll loop goroutine
            ├─ handleDeviceListMessage → fetches device list
            ├─ handleRealMessage     → device status poll response
            │    ├─ MultiPublisher.PublishData()
            │    │    ├─ database.Write()   → PostgreSQL `properties` table
            │    │    └─ mqtt.Write()       → MQTT topics
            │    └─ FeedinController.UpdateFromStatuses()  → caches export_power
            └─ handleDirectMessage   → response to inverter commands
```

The poll loop runs on a configurable interval (`WINET_POLL_INTERVAL`). Inverter commands (charge, discharge, feed-in, etc.) use a single-slot `pendingCmd` channel for request/response correlation — the protocol is serial, one command at a time.

### Amber prices

```
Cron (every 5 min, CRON_TZ=Australia/Adelaide)
  └─ amber.client.GetPrices()
       └─ database.WriteAmberPrices()  → PostgreSQL `amber_prices` table
            └─ FeedinController.Evaluate()  → may call SetFeedInLimitation()
```

### Amber usage

```
Cron (08:00 daily, CRON_TZ=Australia/Adelaide)
  └─ amber.client.GetUsage()  (last 7 days, excluding today)
       └─ database.WriteAmberUsage()  → PostgreSQL `amber_usage` table
```

---

## Services started at runtime

`cmd.Run()` starts the following goroutines via `errgroup`:

| Goroutine | Purpose |
|---|---|
| `startWinetService` | Maintains the WiNet-S WebSocket connection with exponential backoff reconnect (base 5s, max 5m, up to 10 attempts) |
| `startAmberPriceService` | Fetches and stores Amber prices every 5 minutes; triggers feed-in evaluation |
| `startAmberUsageService` | Fetches and stores Amber usage once daily at 08:00 |
| `startHTTPServer` | REST API on `0.0.0.0:8000` |
| `handleErrors` | Drains the error channel; logs cron errors without stopping; fatal errors shut down the process |

---

## Database

PostgreSQL is used for all persistent storage. Schema is managed by [golang-migrate](https://github.com/golang-migrate/migrate) with SQL migration files. Query code is generated by [sqlc](https://sqlc.dev/).

### Tables

| Table | Purpose |
|---|---|
| `devices` | Registered inverter/battery devices |
| `properties` | Time-series device readings (one row per data point per poll) |
| `amber_prices` | Amber price intervals (actual, forecast, current) |
| `amber_usage` | Amber 30-minute usage intervals |
| `users` | API users (bcrypt-hashed passwords) |
| `refresh_tokens` | Active refresh tokens for JWT auth |

### Editing queries

1. Edit the SQL files in `internal/pkg/database/queries/`
2. Run `sqlc generate` to regenerate `internal/pkg/database/db/`
3. Do not edit files in `db/` directly — they are overwritten by sqlc

---

## Code generation

The project uses two code generators. Run them from the repo root:

```bash
go generate ./...
```

This triggers:

- **oapi-codegen** for the REST server (`gen/api.yaml` → `pkg/server/api.gen.go`)
- **oapi-codegen** for the Amber client (`gen/amber/api.json` → `pkg/amber/client.gen.go`)

Mocks are generated by [mockery](https://vektra.github.io/mockery/) using `.mockery.yaml`:

```bash
go run github.com/vektra/mockery/v2
```

---

## HTTP API

The API is defined in [gen/api.yaml](../gen/api.yaml) and served on `:8000`. The server implementation is in [internal/pkg/server/server.go](../internal/pkg/server/server.go).

### Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/auth/login` | None | Login; returns access token + sets `refresh_token` cookie |
| `POST` | `/auth/refresh` | Cookie | Exchange refresh token for new access token |
| `POST` | `/auth/logout` | Cookie | Revoke refresh token and clear cookie |
| `GET` | `/properties` | Bearer | Latest value for every tracked data point |
| `GET` | `/property/{identifier}/{slug}` | Bearer | Time-series for one data point; optional `from`/`to` query params |
| `POST` | `/battery/{state}` | Bearer | Change battery mode: `self_consumption`, `charge`, `discharge`, `stop` |
| `POST` | `/inverter/{state}` | Bearer | Enable (`on`) or disable (`off`) the inverter |
| `POST` | `/inverter/feedin` | Bearer | Enable or disable grid feed-in export |
| `GET` | `/amber/prices/{from}/{to}` | Bearer | Stored Amber prices in a time range |
| `GET` | `/amber/usage/{from}/{to}` | Bearer | Stored Amber usage in a time range |
| `GET` | `/health` | None | Returns `{"status": "connected"|"reconnecting"|"disconnected"|"starting"}` |

### Middleware

- **`TimeoutMiddleware`** — request-scoped context with timeout
- **`LoggingMiddleware`** — request logging + CORS headers
- **`AuthMiddleware`** — validates Bearer token from `Authorization` header; passes through `/auth/*` and `/health`

---

## Authentication

Implemented in [internal/pkg/auth/auth.go](../internal/pkg/auth/auth.go).

- Access tokens are short-lived JWTs (default 15 minutes), sent as `Authorization: Bearer <token>`
- Refresh tokens are long-lived opaque tokens (default 30 days), stored in PostgreSQL and delivered as an `HttpOnly` cookie on `/auth/refresh`
- A background goroutine cleans up expired refresh tokens hourly
- Passwords are stored as bcrypt hashes (`pkg/hasher`)

To create the first user:

```bash
go run ./cmd/createuser
```

---

## Feed-in automation

Implemented in [internal/pkg/feedin/controller.go](../internal/pkg/feedin/controller.go).

The `Controller` is evaluated each time fresh Amber prices are fetched (every ~5 minutes). It enables grid feed-in export if **all** of the following are true:

1. Current time is before 17:30 in the configured timezone
2. At least 10 minutes have elapsed since the last feed-in command (prevents spam)
3. A real-time inverter reading has been received (export power is known)
4. The inverter is not already exporting (`export_power == 0`)
5. The current Amber feed-in price (`channelType == "feedIn"`) is positive

The export power value is kept in memory, updated by `UpdateFromStatuses()` which is called as a hook from the WiNet poll loop.

---

## MQTT publishing

The `MultiPublisher` in [internal/pkg/publisher/publisher.go](../internal/pkg/publisher/publisher.go) fans data out to all registered backends. It includes deduplication: a reading is only written if the value has changed since the last publish (using an in-memory `sync.Map`).

The MQTT backend ([internal/pkg/mqtt/](../internal/pkg/mqtt/)) writes each data point to a topic derived from the device identifier and slug. Topic format is defined in [internal/pkg/mqtt/write.go](../internal/pkg/mqtt/write.go).

---

## Testing

```bash
go test ./...
```

Tests use mockery-generated mocks located in `mocks/`. The mock configuration is in `.mockery.yaml`.

Key test files:

| File | What it tests |
|---|---|
| `internal/pkg/winet/winet_test.go` | WebSocket message handling |
| `internal/pkg/publisher/publisher_test.go` | Deduplication and fan-out logic |
| `internal/pkg/feedin/controller_test.go` | Feed-in evaluation conditions |
| `internal/pkg/auth/auth_test.go` | Token issue, refresh, and revocation |
| `internal/pkg/server/server_test.go` | HTTP handler behaviour |
| `cmd/cmd_test.go` | Amber usage fetch/store orchestration |
| `pkg/sockets/sockets_test.go` | WebSocket connection abstraction |

---

## Local development

### Start dependencies

```bash
docker compose up -d        # PostgreSQL + MQTT
docker compose -f grafana-docker-compose.yml up -d  # optional Grafana on :3000
```

### Environment

Create a `.env` file or export the variables listed in the [configuration reference](#configuration-reference). A minimal example:

```bash
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/winet?sslmode=disable"
export WINET_HOST="192.168.1.x"
export WINET_USERNAME="admin"
export WINET_PASSWORD="yourpassword"
export JWT_SECRET="change-me"
export ALLOWED_ORIGIN="http://localhost:3000"
export AMBER_HOST="https://api.amber.com.au/v1"
export AMBER_TOKEN="your-amber-token"
export SECURE_COOKIES="false"
export LOG_LEVEL="debug"
```

### Run

```bash
go run .
```

### Linting

```bash
golangci-lint run
```

The lint configuration is in [.golangci.yml](../.golangci.yml).
