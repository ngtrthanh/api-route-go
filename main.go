package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
)

type Route struct {
	Callsign     string `json:"callsign"`
	Code         string `json:"code"`
	Number       string `json:"number"`
	AirlineCode  string `json:"airline_code"`
	AirportCodes string `json:"airport_codes"`
}

var routes map[string]Route
var byAirline map[string][]Route
var byAirport map[string][]Route

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

type CountItem struct {
	Code  string `json:"code"`
	Count int    `json:"count"`
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

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func notFound(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(404)
	fmt.Fprintf(w, `{"error":"%s"}`, msg)
}

func paginate(rts []Route, limit, offset int) []Route {
	if offset >= len(rts) {
		return nil
	}
	end := offset + limit
	if end > len(rts) {
		end = len(rts)
	}
	return rts[offset:end]
}

func main() {
	if err := loadRoutes("routes.csv"); err != nil {
		panic(err)
	}
	fmt.Printf("Loaded %d routes, %d airlines, %d airports\n", len(routes), len(byAirline), len(byAirport))

	http.HandleFunc("/v1/routes/", func(w http.ResponseWriter, r *http.Request) {
		cs := r.URL.Path[len("/v1/routes/"):]
		if rt, ok := routes[cs]; ok {
			writeJSON(w, rt)
		} else {
			notFound(w, "Route not found")
		}
	})

	http.HandleFunc("/v1/stats", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"total_routes":   len(routes),
			"total_airlines": len(byAirline),
			"total_airports": len(byAirport),
			"top_airlines":   topN(byAirline, 20),
			"top_airports":   topN(byAirport, 20),
		})
	})

	http.HandleFunc("/v1/airlines/", func(w http.ResponseWriter, r *http.Request) {
		code := strings.ToUpper(r.URL.Path[len("/v1/airlines/"):])
		rts := byAirline[code]
		if len(rts) == 0 {
			notFound(w, "Airline not found")
			return
		}
		limit := qInt(r, "limit", 50, 200)
		offset := qInt(r, "offset", 0, len(rts))
		writeJSON(w, map[string]any{
			"airline":      code,
			"total_routes": len(rts),
			"limit":        limit,
			"offset":       offset,
			"routes":       paginate(rts, limit, offset),
		})
	})

	http.HandleFunc("/v1/airports/", func(w http.ResponseWriter, r *http.Request) {
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
			"airport":            code,
			"total_routes":       len(rts),
			"connected_airports": len(connected),
			"top_connections":    conns,
			"limit":              limit,
			"offset":             offset,
			"routes":             paginate(rts, limit, offset),
		})
	})

	fmt.Println("Listening on :8081")
	http.ListenAndServe(":8081", nil)
}
