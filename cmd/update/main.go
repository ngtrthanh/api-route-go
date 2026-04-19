package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type adsbResponse struct {
	Response struct {
		Flightroute *struct {
			Callsign    string `json:"callsign"`
			CallsignICAO string `json:"callsign_icao"`
			Airline     *struct {
				ICAO string `json:"icao"`
			} `json:"airline"`
			Origin      *struct{ ICAO string `json:"icao_code"` } `json:"origin"`
			Destination *struct{ ICAO string `json:"icao_code"` } `json:"destination"`
		} `json:"flightroute"`
	} `json:"response"`
}

func loadExisting(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return make(map[string]bool), nil // new file
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Read() // skip header
	existing := make(map[string]bool)
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		existing[rec[0]] = true
	}
	return existing, nil
}

func lookup(callsign string) (*adsbResponse, error) {
	resp, err := http.Get("https://api.adsbdb.com/v0/callsign/" + callsign)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var data adsbResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return &data, nil
}

func parseCallsign(cs string) (code, number string) {
	for i, c := range cs {
		if c >= '0' && c <= '9' {
			return cs[:i], cs[i:]
		}
	}
	return cs, ""
}

func main() {
	csvPath := flag.String("csv", "routes.csv", "path to routes.csv")
	inputFile := flag.String("input", "", "file with callsigns to look up (one per line)")
	callsigns := flag.String("callsigns", "", "comma-separated callsigns to look up")
	delay := flag.Duration("delay", 120*time.Millisecond, "delay between API calls")
	flag.Parse()

	existing, err := loadExisting(*csvPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading csv: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Loaded %d existing routes\n", len(existing))

	// Collect callsigns to look up
	var toLookup []string
	if *inputFile != "" {
		data, err := os.ReadFile(*inputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading input: %v\n", err)
			os.Exit(1)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			cs := strings.TrimSpace(strings.ToUpper(line))
			if cs != "" && !existing[cs] {
				toLookup = append(toLookup, cs)
			}
		}
	}
	if *callsigns != "" {
		for _, cs := range strings.Split(*callsigns, ",") {
			cs = strings.TrimSpace(strings.ToUpper(cs))
			if cs != "" && !existing[cs] {
				toLookup = append(toLookup, cs)
			}
		}
	}

	if len(toLookup) == 0 {
		fmt.Println("No new callsigns to look up")
		return
	}
	fmt.Printf("Looking up %d new callsigns\n", len(toLookup))

	// Open CSV for appending
	isNew := len(existing) == 0
	f, err := os.OpenFile(*csvPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening csv: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()

	if isNew {
		w.Write([]string{"callsign", "code", "number", "airline_code", "airport_codes"})
	}

	added, skipped, errors := 0, 0, 0
	for i, cs := range toLookup {
		data, err := lookup(cs)
		if err != nil {
			fmt.Printf("  [%d/%d] %s: error: %v\n", i+1, len(toLookup), cs, err)
			errors++
			time.Sleep(*delay)
			continue
		}
		fr := data.Response.Flightroute
		if fr == nil || fr.Origin == nil || fr.Destination == nil {
			fmt.Printf("  [%d/%d] %s: no route found\n", i+1, len(toLookup), cs)
			skipped++
			time.Sleep(*delay)
			continue
		}

		code, number := parseCallsign(cs)
		airlineCode := code
		if fr.Airline != nil && fr.Airline.ICAO != "" {
			airlineCode = fr.Airline.ICAO
		}
		airports := fr.Origin.ICAO + "-" + fr.Destination.ICAO

		w.Write([]string{cs, code, number, airlineCode, airports})
		fmt.Printf("  [%d/%d] %s: %s -> %s (%s)\n", i+1, len(toLookup), cs, fr.Origin.ICAO, fr.Destination.ICAO, airlineCode)
		added++
		time.Sleep(*delay)
	}

	fmt.Printf("\nDone: %d added, %d skipped, %d errors\n", added, skipped, errors)
}
