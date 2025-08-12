package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type YahooExchanger struct {
	http *http.Client
}

func NewYahooExchanger() *YahooExchanger {
	return &YahooExchanger{http: &http.Client{Timeout: 8 * time.Second}}
}

// Rate returns how many 'to' per 1 'from' using Yahoo chart v8 (e.g., USDTWD=X).
func (y *YahooExchanger) Rate(from, to string) (float64, time.Time, error) {
	from = strings.ToUpper(strings.TrimSpace(from))
	to = strings.ToUpper(strings.TrimSpace(to))
	if from == "" || to == "" {
		return 0, time.Time{}, fmt.Errorf("invalid currency")
	}
	if from == to {
		return 1, time.Now(), nil
	}

	pair := from + to + "=X"
	url := fmt.Sprintf("https://query2.finance.yahoo.com/v8/finance/chart/%s?interval=1h&range=1d", pair)

	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("User-Agent", "stock-portfolios/1.0")
	resp, err := y.http.Do(req)
	if err != nil {
		return 0, time.Time{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, time.Time{}, fmt.Errorf("yahoo fx http %d", resp.StatusCode)
	}

	var raw struct {
		Chart struct {
			Result []struct {
				Meta struct {
					RegularMarketPrice float64 `json:"regularMarketPrice"`
					RegularMarketTime  int64   `json:"regularMarketTime"`
				} `json:"meta"`
			} `json:"result"`
		} `json:"chart"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return 0, time.Time{}, err
	}
	if len(raw.Chart.Result) == 0 {
		return 0, time.Time{}, fmt.Errorf("fx rate not found")
	}
	meta := raw.Chart.Result[0].Meta
	rate := meta.RegularMarketPrice
	asOf := time.Unix(meta.RegularMarketTime, 0)
	if asOf.IsZero() {
		asOf = time.Now()
	}
	if rate <= 0 {
		return 0, time.Time{}, fmt.Errorf("invalid fx rate")
	}
	return rate, asOf, nil
}
