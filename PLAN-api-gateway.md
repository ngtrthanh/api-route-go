# api.hpradar.com — Unified API Gateway Routing Plan

## URL Structure

```
api.hpradar.com
├── /                           → API index (built-in)
│
├── /v0/aircraft/{mode_s|reg}   → built-in: aircraft lookup (516k)
├── /v0/callsign/{callsign}    → built-in: flight route + airline + airports (474k)
├── /v0/airline/{icao}          → built-in: airline info (5.8k)
├── /v0/n-number/{n}            → built-in: N-number → Mode-S
├── /v0/mode-s/{hex}            → built-in: Mode-S → registration
├── /v0/online                  → built-in: health/uptime
│
├── /v1/routes/{callsign}       → built-in: compact route lookup
├── /v1/stats                   → built-in: database overview
├── /v1/airlines/{icao}         → built-in: paginated airline routes
├── /v1/airports/{icao}         → built-in: paginated airport routes
│
├── /adsb/*                     → proxy → :8332 (proxy-adsbx-go, live ADS-B feed)
├── /meteo/*                    → proxy → :8113 (hydro-api, tide/weather/maritime)
│   ├── /meteo/api/stations
│   ├── /meteo/api/meteo
│   ├── /meteo/api/hydro
│   ├── /meteo/api/tidal/predict/{mmsi}
│   └── /meteo/api/health
│
├── /shipment/*                 → proxy → :8112 (parser-api, shipment parsing)
│   ├── /shipment/parse
│   ├── /shipment/parse-batch
│   └── /shipment/db-stats-prod
│
├── /geo/*                      → proxy → :3001 (country-geocoder)
│
├── /coal/*                     → proxy → :8448 (coal-monitor)
├── /limestone/*                → proxy → :8450 (limestone-monitor)
│
├── /ais/*                      → proxy → :8200 (ais-infra, AIS receiver web)
├── /sea/*                      → proxy → :8527 (sea, AIS maritime)
│
└── /skylink/*                  → proxy → :41180 (skylink-core API)
```

## What This Replaces

| Old Subdomain       | New Path              | Backend        |
|---------------------|-----------------------|----------------|
| api8332.hpradar.com | api.hpradar.com/adsb  | :8332          |
| tide.hpradar.com    | api.hpradar.com/meteo | :8113          |
| sea.hpradar.com     | api.hpradar.com/sea   | :8527          |
| aisinfra.hpradar.com| api.hpradar.com/ais   | :8200          |
| (new)               | api.hpradar.com/v0/*  | built-in       |
| (new)               | api.hpradar.com/v1/*  | built-in       |
| api8335-8339        | api.hpradar.com/adsb  | consolidated   |

## Features (built into Go binary)

- Rate limiting: 600 req/min per IP (all paths)
- CORS: permissive for browser clients
- Request logging: IP, method, path, status, latency
- Reverse proxy: strips prefix, forwards to backend
- CF tunnel aware: reads CF-Connecting-IP

## Implementation

Single Go binary handles:
1. All /v0/* and /v1/* endpoints directly (in-memory, ~17k req/s)
2. All other paths reverse-proxied to backend services (near-zero overhead)
3. Middleware applied uniformly to all traffic
