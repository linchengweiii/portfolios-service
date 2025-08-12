package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Yahoo Finance v8 chart provider (cached)

var ErrYahooNoResult = errors.New("yahoo: no result")

type YahooProvider struct {
	cli   *http.Client
	ttl   time.Duration
	mu    sync.RWMutex
	cache map[string]cachedQuote
}

func NewYahooProvider() *YahooProvider {
	return &YahooProvider{
		cli:   &http.Client{Timeout: 8 * time.Second},
		ttl:   60 * time.Second,
		cache: make(map[string]cachedQuote),
	}
}

func (p *YahooProvider) GetPrice(symbol string) (float64, time.Time, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return 0, time.Time{}, ErrPriceNotFound
	}

	// Cache
	p.mu.RLock()
	if c, ok := p.cache[symbol]; ok && time.Since(c.fetched) < p.ttl {
		p.mu.RUnlock()
		return c.price, c.asOf, nil
	}
	p.mu.RUnlock()

	url := fmt.Sprintf("https://query2.finance.yahoo.com/v8/finance/chart/%s?interval=1m&range=1d", symbol)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("User-Agent", "stock-portfolios/1.0")

	resp, err := p.cli.Do(req)
	if err != nil {
		return 0, time.Time{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, time.Time{}, fmt.Errorf("yahoo http %d", resp.StatusCode)
	}

	var raw struct {
		Chart struct {
			Result []struct {
				Meta struct {
					RegularMarketPrice float64 `json:"regularMarketPrice"`
					RegularMarketTime  int64   `json:"regularMarketTime"`
				} `json:"meta"`
				Timestamp  []int64 `json:"timestamp"`
				Indicators struct {
					Quote []struct {
						Close []float64 `json:"close"`
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
			Error any `json:"error"`
		} `json:"chart"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return 0, time.Time{}, err
	}
	if len(raw.Chart.Result) == 0 {
		return 0, time.Time{}, ErrYahooNoResult
	}

	r := raw.Chart.Result[0]
	price := r.Meta.RegularMarketPrice
	asOf := time.Unix(r.Meta.RegularMarketTime, 0)

	// Fallback: last non-zero close if meta missing
	if (price <= 0 || r.Meta.RegularMarketTime == 0) && len(r.Timestamp) > 0 && len(r.Indicators.Quote) > 0 && len(r.Indicators.Quote[0].Close) == len(r.Timestamp) {
		for i := len(r.Timestamp) - 1; i >= 0; i-- {
			c := r.Indicators.Quote[0].Close[i]
			if c > 0 {
				price = c
				asOf = time.Unix(r.Timestamp[i], 0)
				break
			}
		}
	}

	if price <= 0 {
		return 0, time.Time{}, ErrPriceNotFound
	}
	if asOf.IsZero() {
		asOf = time.Now()
	}

	p.mu.Lock()
	p.cache[symbol] = cachedQuote{price: price, asOf: asOf, fetched: time.Now()}
	p.mu.Unlock()

	return price, asOf, nil
}
