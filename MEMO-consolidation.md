# Memo: Consolidating APIs under api.hpradar.com

**Date:** 2026-04-19
**Author:** Kiro CLI

---

## Current State

### Running HTTP API Services (hpradar-related)

| Service | Container | Port | Function | Status |
|---------|-----------|------|----------|--------|
| **api-route-go** | publicapi-route-go-api-route-go-1 | :5737 | Aircraft routes, callsign/airline/airport lookup (adsbdb-compatible) | ✅ Active |
| **proxy-adsbx-go** | proxy-adsbx-go | :8332 | ADS-B Exchange proxy (Go) | ✅ Active |
| **proxy-lol-rust** | proxy-lol-rust | :8335 | ADS-B proxy (Rust) | ✅ Active |
| **aircraft_proxy_rs** | aircraft_proxy_rs | :8339 | Aircraft data proxy (Rust) | ✅ Active |
| **proxy-lol-bin** | proxy-lol-bin | :8338 | ADS-B proxy (binary) | ✅ Active |
| **hydro-api** | hydro-api | :8113 | Tide/maritime meteo data | ✅ Active |
| **parser-api** | parser-api | :8112 | Shipment parser | ✅ Active |
| **flask_api_p** | flask_api_p | :5566 | Live materials dashboard | ✅ Active |
| **health_api** | health_api | :8558 | Health monitoring (nginx) | ✅ Active |
| **acgeo** | acgeo-acgeo-1 | :5000 | Aircraft geolocation | ✅ Active |
| **country-geocoder** | country-geocoder-country-geocoder-1 | :3001 | Lat/lon → country lookup | ✅ Active |
| **hpr-globe** | hpr-globe-web-1 | :7777 | HPRadar Ops Dashboard (web) | ✅ Active |
| **skylink** | skylink | :31787 | tar1090 ADS-B web viewer | ✅ Active |
| **skylink-core** | skylink-core-staging | :10111,:41180 | Core ADS-B aggregation | ✅ Active |
| **sea** | sea | :8527 | AIS maritime receiver | ✅ Active |
| **ais-infra** | ais-infra | :8200 | AIS infrastructure | ✅ Active |
| **coal-monitor** | coal-monitor | :8448 | Coal tracking | ✅ Active |
| **limestone-monitor** | limestone-monitor | :8450 | Limestone tracking | ✅ Active |

### Existing CF Tunnel Subdomains (33 active)

`a01` `a1` `a2` `acapi` `acm` `adsb04` `aibot` `air18` `aisinfra` `api` `api8332` `api8333` `api8334` `api8335` `api8336` `api8339` `api8989` `bctb` `cfcfleet` `chex` `health` `legacy` `m1` `meteo` `mlat` `n8n` `sea` `skylink` `stamon` `tide` `ts` `whd` `worldmonitor`

### Infrastructure

| Component | Container | Purpose |
|-----------|-----------|---------|
| Caddy | caddy | Reverse proxy, TLS (ports 80/443) |
| Traefik | traefik-traefik-1 | Reverse proxy (ports 7080/7443) |
| Kong Gateway | kong-gateway | API gateway (port 8600) |
| Cloudflare Tunnel | cloudflared-tunnel | Tunnel to CF (token-based) |
| Cloudflare DDNS | cloudflared-ddns | Dynamic DNS |

---

## Consolidation Plan: api.hpradar.com

### Option A: Path-based routing (Recommended)

Route all aviation/data APIs through `api.hpradar.com` with path prefixes:

```
api.hpradar.com/v0/aircraft/*      → api-route-go (:5737)   — aircraft lookup
api.hpradar.com/v0/callsign/*      → api-route-go (:5737)   — flight routes
api.hpradar.com/v0/airline/*        → api-route-go (:5737)   — airline info
api.hpradar.com/v0/n-number/*       → api-route-go (:5737)   — N-number lookup
api.hpradar.com/v0/mode-s/*         → api-route-go (:5737)   — Mode-S lookup
api.hpradar.com/v0/online           → api-route-go (:5737)   — health check
api.hpradar.com/v1/*                → api-route-go (:5737)   — extended endpoints

api.hpradar.com/adsb/*              → proxy-adsbx-go (:8332) — live ADS-B data
api.hpradar.com/aircraft-proxy/*    → aircraft_proxy_rs (:8339)
api.hpradar.com/geocode/*           → country-geocoder (:3001)
api.hpradar.com/meteo/*             → hydro-api (:8113)      — tide/weather
api.hpradar.com/shipment/*          → parser-api (:8112)     — shipment parsing
```

**Implementation:** Kong Gateway is already running on :8600 and `api.hpradar.com` already points to it via CF tunnel. Just add routes in Kong.

### Option B: Subdomain-based (Current approach)

Keep separate subdomains but standardize naming:

```
routes.hpradar.com    → api-route-go (:5737)
adsb.hpradar.com      → proxy-adsbx-go (:8332)   (currently api8332)
meteo.hpradar.com     → hydro-api (:8113)         (currently tide)
geo.hpradar.com       → country-geocoder (:3001)
```

---

## Recommendation

**Go with Option A** — use Kong Gateway + `api.hpradar.com`:

1. `api.hpradar.com` already exists in CF tunnel → Kong (:8600)
2. Kong is already running and healthy
3. Single domain, single TLS cert, single CORS config
4. Path-based routing is cleaner for API consumers
5. Eliminates the `api8332`, `api8335`, etc. numbered subdomain mess

### Quick wins to clean up:
- Remove `api8332`–`api8339` subdomains, consolidate under `api.hpradar.com/adsb/`
- Point `api.hpradar.com/v0/*` and `/v1/*` to api-route-go
- Retire the Python api-route container (:5735) — Go version replaces it
- Retire the lab api-route-go container (:5736) — production is on :5737

### Steps to implement:
1. Add Kong routes: `api.hpradar.com` → path-based upstream services
2. Test all paths through Kong
3. Update CF tunnel config to remove old numbered subdomains
4. Update any clients/frontends using old endpoints
5. Shut down redundant containers
