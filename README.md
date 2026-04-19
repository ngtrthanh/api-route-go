# Aircraft Routes API

A high-performance REST API for querying aircraft flight routes by callsign, airline, or airport. Built with Go and backed by an in-memory dataset of 474k+ routes.

## Quick Start

```bash
docker compose up -d --build
```

The API will be available at `http://localhost:8081`.

## Endpoints

### Get Route by Callsign

```
GET /v1/routes/{callsign}
```

Look up a specific flight route by its ICAO callsign.

**Example:**

```bash
curl http://localhost:8081/v1/routes/BAW123
```

```json
{
  "callsign": "BAW123",
  "code": "BAW",
  "number": "123",
  "airline_code": "BAW",
  "airport_codes": "EGLL-KJFK"
}
```

**Response codes:**
- `200` — Route found
- `404` — `{"error": "Route not found"}`

---

### Database Statistics

```
GET /v1/stats
```

Returns an overview of the database including totals and top 20 airlines/airports by route count.

**Example:**

```bash
curl http://localhost:8081/v1/stats
```

```json
{
  "total_routes": 474099,
  "total_airlines": 1416,
  "total_airports": 3548,
  "top_airlines": [
    { "code": "RYR", "count": 36765 },
    { "code": "EZY", "count": 26558 }
  ],
  "top_airports": [
    { "code": "LFPG", "count": 15528 },
    { "code": "EHAM", "count": 13254 }
  ]
}
```

---

### Get Routes by Airline

```
GET /v1/airlines/{icao_code}?limit=50&offset=0
```

Returns all routes operated by an airline (ICAO code). Results are paginated.

| Parameter | Default | Max | Description |
|-----------|---------|-----|-------------|
| `limit`   | 50      | 200 | Routes per page |
| `offset`  | 0       | —   | Skip N routes |

**Example:**

```bash
curl "http://localhost:8081/v1/airlines/RYR?limit=5&offset=0"
```

```json
{
  "airline": "RYR",
  "total_routes": 36765,
  "limit": 5,
  "offset": 0,
  "routes": [
    {
      "callsign": "RYR10",
      "code": "RYR",
      "number": "10",
      "airline_code": "RYR",
      "airport_codes": "EGNX-EGPK"
    }
  ]
}
```

**Response codes:**
- `200` — Airline found
- `404` — `{"error": "Airline not found"}`

---

### Get Routes by Airport

```
GET /v1/airports/{icao_code}?limit=50&offset=0
```

Returns all routes through an airport, plus a ranked list of connected airports.

| Parameter | Default | Max | Description |
|-----------|---------|-----|-------------|
| `limit`   | 50      | 200 | Routes per page |
| `offset`  | 0       | —   | Skip N routes |

**Example:**

```bash
curl "http://localhost:8081/v1/airports/EGLL?limit=3"
```

```json
{
  "airport": "EGLL",
  "total_routes": 6673,
  "connected_airports": 338,
  "top_connections": [
    { "code": "LFPG", "count": 256 },
    { "code": "EGPH", "count": 174 },
    { "code": "EDDF", "count": 166 }
  ],
  "limit": 3,
  "offset": 0,
  "routes": [
    {
      "callsign": "BAW1",
      "code": "BAW",
      "number": "1",
      "airline_code": "BAW",
      "airport_codes": "EGLL-KJFK"
    }
  ]
}
```

**Response codes:**
- `200` — Airport found
- `404` — `{"error": "Airport not found"}`

---

## Data Fields

| Field | Description | Example |
|-------|-------------|---------|
| `callsign` | ICAO flight callsign | `BAW123` |
| `code` | Airline prefix from callsign | `BAW` |
| `number` | Flight number portion | `123` |
| `airline_code` | ICAO airline code | `BAW` |
| `airport_codes` | Origin-Destination ICAO pair | `EGLL-KJFK` |

## Update Tool

A CLI tool to add new routes by querying the [adsbdb.com](https://www.adsbdb.com/) API.

```bash
# Build
go build -o update-tool ./cmd/update/

# Look up specific callsigns
./update-tool -callsigns "RYR99ZZ,BAW456"

# Bulk from file (one callsign per line)
./update-tool -input new_callsigns.txt

# Custom rate limit (default 120ms ≈ 500 req/min)
./update-tool -input new_callsigns.txt -delay 200ms
```

After updating, restart the container to load new data:

```bash
docker compose restart
```

## Performance

Benchmarked with Apache Bench on 16-core host:

| Concurrency | Requests/sec | Median Latency | p99 Latency |
|-------------|-------------|----------------|-------------|
| 10          | 18,312      | <1ms           | 2ms         |
| 50          | 16,278      | 3ms            | 4ms         |
| 100         | 16,449      | 6ms            | 10ms        |

## Docker

```bash
# Start
docker compose up -d --build

# Logs
docker compose logs -f

# Stop
docker compose down
```

## License

Data sourced from [adsbdb.com](https://www.adsbdb.com/). Flight route data by David Taylor (Edinburgh) and Jim Mason (Glasgow).
