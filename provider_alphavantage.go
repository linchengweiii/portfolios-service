package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Alpha Vantage GLOBAL_QUOTE provider (simple, cached)

var (
	ErrPriceNotFound  = errors.New("price not found")
	ErrAPIKeyMissing  = errors.New("ALPHAVANTAGE_API_KEY not set")
	ErrAPIRateLimited = errors.New("alpha vantage rate limit or information note")
)

type AlphaVantageProvider struct {
	apiKey string
	cli    *http.Client
	ttl    time.Duration

	mu    sync.RWMutex
	cache map[string]cachedQuote
}

type cachedQuote struct {
	price   float64
	asOf    time.Time
	fetched time.Time
}

func NewAlphaVantageProviderFromEnv() (*AlphaVantageProvider, error) {
	key := strings.TrimSpace(os.Getenv("ALPHAVANTAGE_API_KEY"))
	if key == "" {
		return nil, ErrAPIKeyMissing
	}
	return &AlphaVantageProvider{
		apiKey: key,
		cli:    &http.Client{Timeout: 8 * time.Second},
		ttl:    60 * time.Second,
		cache:  make(map[string]cachedQuote),
	}, nil
}

func (p *AlphaVantageProvider) GetPrice(symbol string) (float64, time.Time, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return 0, time.Time{}, ErrPriceNotFound
	}

	// cache hit?
	p.mu.RLock()
	if c, ok := p.cache[symbol]; ok && time.Since(c.fetched) < p.ttl {
		p.mu.RUnlock()
		return c.price, c.asOf, nil
	}
	p.mu.RUnlock()

	url := fmt.Sprintf("https://www.alphavantage.co/query?function=GLOBAL_QUOTE&symbol=%s&apikey=%s", symbol, p.apiKey)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("User-Agent", "stock-portfolios/1.0")

	resp, err := p.cli.Do(req)
	if err != nil {
		return 0, time.Time{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, time.Time{}, fmt.Errorf("alphavantage http %d", resp.StatusCode)
	}

	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return 0, time.Time{}, err
	}
	if _, ok := raw["Note"]; ok {
		return 0, time.Time{}, ErrAPIRateLimited
	}
	if _, ok := raw["Information"]; ok {
		return 0, time.Time{}, ErrAPIRateLimited
	}
	gq, ok := raw["Global Quote"].(map[string]any)
	if !ok || len(gq) == 0 {
		return 0, time.Time{}, ErrPriceNotFound
	}

	priceStr, _ := gq["05. price"].(string)
	asOfStr, _ := gq["07. latest trading day"].(string)

	price, err := strconv.ParseFloat(priceStr, 64)
	if err != nil || price <= 0 {
		return 0, time.Time{}, ErrPriceNotFound
	}

	asOf := time.Now()
	if asOfStr != "" {
		if t, e := time.Parse("2006-01-02", asOfStr); e == nil {
			asOf = t
		}
	}

	p.mu.Lock()
	p.cache[symbol] = cachedQuote{price: price, asOf: asOf, fetched: time.Now()}
	p.mu.Unlock()

	return price, asOf, nil
}
