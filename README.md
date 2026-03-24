# winet-integration

A Go service that integrates with the [Sungrow WiNet-S](https://en.sungrowpower.com/productDetail/2893/winet-s) Wi-Fi dongle to monitor and control Sungrow inverters and batteries. It also integrates with [Amber Electric](https://www.amber.com.au/) to track live electricity prices and automate feed-in export decisions.

## What it does

- **Inverter polling** — connects to the WiNet-S dongle over WebSocket and polls real-time device data (power, battery state, export power, etc.) on a configurable interval
- **Data storage** — persists device readings to PostgreSQL via generated SQL (sqlc)
- **MQTT publishing** — fans out sensor readings to an MQTT broker (e.g. for Home Assistant)
- **Amber Electric integration** — fetches live electricity prices every 5 minutes and historical usage daily, storing both in PostgreSQL
- **Automated feed-in control** — enables grid export automatically when the Amber feed-in price is positive (before 5:30 PM), with a 10-minute command TTL to prevent spam
- **REST API** — HTTP server on port `8000` for querying data and sending inverter/battery commands
- **JWT authentication** — access + refresh token auth for the API

## Quick start

### Prerequisites

- Go 1.23+
- Docker (for PostgreSQL and MQTT)

### Run infrastructure

```bash
docker compose up -d
```

This starts PostgreSQL (port 5432) and Mosquitto MQTT (ports 1883/9001).

### Configure

All configuration is via environment variables. Required variables:

| Variable | Description |
|---|---|
| `DATABASE_URL` | PostgreSQL DSN |
| `WINET_HOST` | WiNet-S device IP or hostname |
| `WINET_USERNAME` | WiNet-S username |
| `WINET_PASSWORD` | WiNet-S password |
| `JWT_SECRET` | Secret for signing JWTs |
| `ALLOWED_ORIGIN` | Comma-separated allowed CORS origins |
| `AMBER_HOST` | Amber API base URL (e.g. `https://api.amber.com.au/v1`) |
| `AMBER_TOKEN` | Amber API token |

See [docs/developer.md](docs/developer.md) for the full configuration reference and architecture details.

### Run

```bash
go run .
```

### Create a user

```bash
go run ./cmd/createuser
```

## API

The REST API is defined in [gen/api.yaml](gen/api.yaml). Key endpoints:

- `POST /auth/login` — obtain access + refresh tokens
- `GET /properties` — latest inverter readings
- `GET /property/{identifier}/{slug}` — time-series readings for a specific data point
- `POST /battery/{state}` — set battery mode (`self_consumption`, `charge`, `discharge`, `stop`)
- `POST /inverter/{state}` — enable or disable the inverter (`on`/`off`)
- `POST /inverter/feedin` — enable or disable grid feed-in export
- `GET /amber/prices/{from}/{to}` — stored Amber price history
- `GET /amber/usage/{from}/{to}` — stored Amber usage history
- `GET /health` — WiNet connection health status

## Visualisation

A Grafana instance can be started with:

```bash
docker compose -f grafana-docker-compose.yml up -d
```
