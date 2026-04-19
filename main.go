package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ===================== MIDDLEWARE =====================

// --- Rate Limiter (per-IP, sliding window) ---

type visitor struct {
	count    int
	resetAt  time.Time
}

type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	limit    int
	window   time.Duration
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{visitors: make(map[string]*visitor), limit: limit, window: window}
	go func() {
		for range time.Tick(window) {
			rl.mu.Lock()
			now := time.Now()
			for k, v := range rl.visitors {
				if now.After(v.resetAt) {
					delete(rl.visitors, k)
				}
			}
			rl.mu.Unlock()
		}
	}()
	return rl
}

func (rl *RateLimiter) Allow(ip string) (bool, int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	v, ok := rl.visitors[ip]
	if !ok || now.After(v.resetAt) {
		rl.visitors[ip] = &visitor{count: 1, resetAt: now.Add(rl.window)}
		return true, rl.limit - 1
	}
	v.count++
	if v.count > rl.limit {
		return false, 0
	}
	return true, rl.limit - v.count
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.Split(xff, ",")[0]
	}
	if xri := r.Header.Get("CF-Connecting-IP"); xri != "" {
		return xri
	}
	return strings.Split(r.RemoteAddr, ":")[0]
}

// --- Middleware chain ---

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func rateLimit(rl *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			ok, remaining := rl.Allow(ip)
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			if !ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(429)
				w.Write([]byte(`{"error":"rate limit exceeded"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		log.Printf("%s %s %s %d %s", clientIP(r), r.Method, r.URL.Path, sw.status, time.Since(start).Round(time.Microsecond))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func chain(h http.Handler, mw ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}

// ===================== DATA TYPES =====================

type Aircraft struct {
	ModeS        string `json:"mode_s"`
	Registration string `json:"registration"`
	ICAOType     string `json:"icao_type"`
	ShortType    string `json:"short_type,omitempty"`
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	Owner        string `json:"registered_owner"`
	Year         string `json:"year,omitempty"`
	Mil          bool   `json:"military"`
	PIA          bool   `json:"faa_pia"`
	LADD         bool   `json:"faa_ladd"`
}

type Airline struct {
	Name     string `json:"name"`
	ICAO     string `json:"icao"`
	IATA     string `json:"iata"`
	Country  string `json:"country"`
	Callsign string `json:"callsign"`
}

type Airport struct {
	ICAO      string  `json:"icao_code"`
	IATA      string  `json:"iata_code"`
	Name      string  `json:"name"`
	City      string  `json:"municipality"`
	Country   string  `json:"country_name"`
	Lat       float64 `json:"latitude"`
	Lon       float64 `json:"longitude"`
	Elevation int     `json:"elevation"`
}

type Route struct {
	Callsign     string `json:"callsign"`
	Code         string `json:"code"`
	Number       string `json:"number"`
	AirlineCode  string `json:"airline_code"`
	AirportCodes string `json:"airport_codes"`
}

type FlightRoute struct {
	Callsign     string   `json:"callsign"`
	CallsignICAO string   `json:"callsign_icao"`
	Airline      *Airline `json:"airline"`
	Origin       *Airport `json:"origin"`
	Destination  *Airport `json:"destination"`
}

type CountItem struct {
	Code  string `json:"code"`
	Count int    `json:"count"`
}

// ===================== DATA STORES =====================

var (
	aircraft    map[string]Aircraft
	regToModeS  map[string]string
	nNumToModeS map[string]string
	airlines    map[string]Airline
	airports    map[string]Airport
	routes      map[string]Route
	byAirline   map[string][]Route
	byAirport   map[string][]Route
	startTime   time.Time
)

// ===================== LOADERS =====================

func loadAircraft(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Read()
	aircraft = make(map[string]Aircraft, 620000)
	regToModeS = make(map[string]string, 620000)
	nNumToModeS = make(map[string]string, 300000)
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		ac := Aircraft{
			ModeS: rec[0], Registration: rec[1], ICAOType: rec[2], ShortType: rec[3],
			Manufacturer: rec[4], Model: rec[5], Owner: rec[6], Year: rec[7],
			Mil: rec[8] == "1", PIA: rec[9] == "1", LADD: rec[10] == "1",
		}
		aircraft[rec[0]] = ac
		regToModeS[rec[1]] = rec[0]
		if strings.HasPrefix(rec[1], "N") {
			nNumToModeS[rec[1]] = rec[0]
		}
	}
	return nil
}

func loadAirlines(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Read()
	airlines = make(map[string]Airline, 6000)
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		airlines[rec[1]] = Airline{Name: rec[0], ICAO: rec[1], IATA: rec[2], Country: rec[3], Callsign: rec[4]}
	}
	return nil
}

func loadAirports(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Read()
	airports = make(map[string]Airport, 8000)
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		lat, _ := strconv.ParseFloat(rec[5], 64)
		lon, _ := strconv.ParseFloat(rec[6], 64)
		elev, _ := strconv.Atoi(rec[7])
		airports[rec[0]] = Airport{ICAO: rec[0], IATA: rec[1], Name: rec[2], City: rec[3], Country: rec[4], Lat: lat, Lon: lon, Elevation: elev}
	}
	return nil
}

func loadRoutes(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Read()
	routes = make(map[string]Route, 500000)
	byAirline = make(map[string][]Route)
	byAirport = make(map[string][]Route)
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		rt := Route{rec[0], rec[1], rec[2], rec[3], rec[4]}
		routes[rec[0]] = rt
		byAirline[rt.AirlineCode] = append(byAirline[rt.AirlineCode], rt)
		for _, ap := range strings.Split(rt.AirportCodes, "-") {
			byAirport[ap] = append(byAirport[ap], rt)
		}
	}
	return nil
}

// ===================== HELPERS =====================

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func notFound(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(404)
	fmt.Fprintf(w, `{"response":"%s"}`, msg)
}

func qInt(r *http.Request, key string, def, max int) int {
	v, _ := strconv.Atoi(r.URL.Query().Get(key))
	if v <= 0 {
		return def
	}
	if v > max {
		return max
	}
	return v
}

func topN(m map[string][]Route, n int) []CountItem {
	items := make([]CountItem, 0, len(m))
	for k, v := range m {
		items = append(items, CountItem{k, len(v)})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Count > items[j].Count })
	if len(items) > n {
		items = items[:n]
	}
	return items
}

func paginate(rts []Route, limit, offset int) []Route {
	if offset >= len(rts) {
		return []Route{}
	}
	end := offset + limit
	if end > len(rts) {
		end = len(rts)
	}
	return rts[offset:end]
}

// ===================== MAIN =====================

func main() {
	for _, l := range []struct {
		name string
		fn   func(string) error
		path string
	}{
		{"aircraft", loadAircraft, "aircraft.csv"},
		{"airlines", loadAirlines, "airlines.csv"},
		{"airports", loadAirports, "airports.csv"},
		{"routes", loadRoutes, "routes.csv"},
	} {
		if err := l.fn(l.path); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load %s: %v\n", l.name, err)
			os.Exit(1)
		}
	}
	startTime = time.Now()
	fmt.Printf("Loaded: %d aircraft, %d airlines, %d airports, %d routes\n",
		len(aircraft), len(airlines), len(airports), len(routes))

	mux := http.NewServeMux()

	// === adsbdb-compatible v0 ===

	mux.HandleFunc("/v0/aircraft/", func(w http.ResponseWriter, r *http.Request) {
		key := strings.ToUpper(r.URL.Path[len("/v0/aircraft/"):])
		ac, ok := aircraft[key]
		if !ok {
			if ms, found := regToModeS[key]; found {
				ac, ok = aircraft[ms]
			}
		}
		if ok {
			writeJSON(w, map[string]any{"response": map[string]any{"aircraft": ac}})
		} else {
			notFound(w, "unknown aircraft")
		}
	})

	mux.HandleFunc("/v0/callsign/", func(w http.ResponseWriter, r *http.Request) {
		cs := strings.ToUpper(r.URL.Path[len("/v0/callsign/"):])
		rt, ok := routes[cs]
		if !ok {
			notFound(w, "unknown callsign")
			return
		}
		fr := FlightRoute{Callsign: cs, CallsignICAO: cs}
		if al, ok := airlines[rt.AirlineCode]; ok {
			fr.Airline = &al
		}
		parts := strings.Split(rt.AirportCodes, "-")
		if len(parts) >= 2 {
			if ap, ok := airports[parts[0]]; ok {
				fr.Origin = &ap
			}
			if ap, ok := airports[parts[1]]; ok {
				fr.Destination = &ap
			}
		}
		writeJSON(w, map[string]any{"response": map[string]any{"flightroute": fr}})
	})

	mux.HandleFunc("/v0/airline/", func(w http.ResponseWriter, r *http.Request) {
		code := strings.ToUpper(r.URL.Path[len("/v0/airline/"):])
		if al, ok := airlines[code]; ok {
			writeJSON(w, map[string]any{"response": []Airline{al}})
		} else {
			notFound(w, "unknown airline")
		}
	})

	mux.HandleFunc("/v0/n-number/", func(w http.ResponseWriter, r *http.Request) {
		n := strings.ToUpper(r.URL.Path[len("/v0/n-number/"):])
		if !strings.HasPrefix(n, "N") {
			n = "N" + n
		}
		if ms, ok := nNumToModeS[n]; ok {
			writeJSON(w, map[string]any{"response": ms})
		} else {
			notFound(w, "unknown n-number")
		}
	})

	mux.HandleFunc("/v0/mode-s/", func(w http.ResponseWriter, r *http.Request) {
		hex := strings.ToUpper(r.URL.Path[len("/v0/mode-s/"):])
		if ac, ok := aircraft[hex]; ok {
			writeJSON(w, map[string]any{"response": ac.Registration})
		} else {
			notFound(w, "unknown mode-s")
		}
	})

	mux.HandleFunc("/v0/online", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"response": map[string]any{
			"uptime":      int(time.Since(startTime).Seconds()),
			"api_version": "1.0.0",
		}})
	})

	// === Extended v1 ===

	mux.HandleFunc("/v1/routes/", func(w http.ResponseWriter, r *http.Request) {
		cs := strings.ToUpper(r.URL.Path[len("/v1/routes/"):])
		if rt, ok := routes[cs]; ok {
			writeJSON(w, rt)
		} else {
			notFound(w, "Route not found")
		}
	})

	mux.HandleFunc("/v1/stats", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"total_aircraft": len(aircraft),
			"total_routes":   len(routes),
			"total_airlines": len(byAirline),
			"total_airports": len(byAirport),
			"top_airlines":   topN(byAirline, 20),
			"top_airports":   topN(byAirport, 20),
		})
	})

	mux.HandleFunc("/v1/airlines/", func(w http.ResponseWriter, r *http.Request) {
		code := strings.ToUpper(r.URL.Path[len("/v1/airlines/"):])
		rts := byAirline[code]
		if len(rts) == 0 {
			notFound(w, "Airline not found")
			return
		}
		limit := qInt(r, "limit", 50, 200)
		offset := qInt(r, "offset", 0, len(rts))
		writeJSON(w, map[string]any{
			"airline": code, "total_routes": len(rts),
			"limit": limit, "offset": offset,
			"routes": paginate(rts, limit, offset),
		})
	})

	mux.HandleFunc("/v1/airports/", func(w http.ResponseWriter, r *http.Request) {
		code := strings.ToUpper(r.URL.Path[len("/v1/airports/"):])
		rts := byAirport[code]
		if len(rts) == 0 {
			notFound(w, "Airport not found")
			return
		}
		connected := make(map[string]int)
		for _, rt := range rts {
			for _, p := range strings.Split(rt.AirportCodes, "-") {
				if p != code {
					connected[p]++
				}
			}
		}
		conns := make([]CountItem, 0, len(connected))
		for k, v := range connected {
			conns = append(conns, CountItem{k, v})
		}
		sort.Slice(conns, func(i, j int) bool { return conns[i].Count > conns[j].Count })
		limit := qInt(r, "limit", 50, 200)
		offset := qInt(r, "offset", 0, len(rts))
		writeJSON(w, map[string]any{
			"airport": code, "total_routes": len(rts),
			"connected_airports": len(connected), "top_connections": conns,
			"limit": limit, "offset": offset,
			"routes": paginate(rts, limit, offset),
		})
	})

	// === Apply middleware: logging → CORS → rate limit → mux ===
	rl := NewRateLimiter(600, time.Minute) // 600 req/min per IP
	handler := chain(mux, requestLog, cors, rateLimit(rl))

	fmt.Println("Listening on :8081")
	log.Fatal(http.ListenAndServe(":8081", handler))
}
