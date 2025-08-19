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
    hist  map[string]histSeries
}

func NewYahooProvider() *YahooProvider {
    return &YahooProvider{
        cli:   &http.Client{Timeout: 8 * time.Second},
        ttl:   60 * time.Second,
        cache: make(map[string]cachedQuote),
        hist:  make(map[string]histSeries),
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

// ---- Historical daily prices ----

type histSeries struct {
    days    []time.Time
    closes  []float64
    opens   []float64
    fetched time.Time
}

func (p *YahooProvider) GetPriceOn(symbol string, date time.Time) (float64, time.Time, error) {
    symbol = strings.ToUpper(strings.TrimSpace(symbol))
    if symbol == "" {
        return 0, time.Time{}, ErrPriceNotFound
    }
    date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)

    // cache hit
    p.mu.RLock()
    hs, ok := p.hist[symbol]
    if ok && time.Since(hs.fetched) < p.ttl && len(hs.days) > 0 {
        p.mu.RUnlock()
        return lookupHistClose(hs, date)
    }
    p.mu.RUnlock()

    // fetch range daily for up to 10y
    url := fmt.Sprintf("https://query2.finance.yahoo.com/v8/finance/chart/%s?interval=1d&range=10y", symbol)
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
                Timestamp  []int64 `json:"timestamp"`
                Indicators struct {
                    Quote []struct {
                        Open  []float64 `json:"open"`
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
    if len(r.Timestamp) == 0 || len(r.Indicators.Quote) == 0 || len(r.Indicators.Quote[0].Close) != len(r.Timestamp) {
        return 0, time.Time{}, ErrPriceNotFound
    }
    days := make([]time.Time, 0, len(r.Timestamp))
    closes := make([]float64, 0, len(r.Timestamp))
    opens := make([]float64, 0, len(r.Timestamp))
    for i := 0; i < len(r.Timestamp); i++ {
        ts := time.Unix(r.Timestamp[i], 0).UTC()
        c := r.Indicators.Quote[0].Close[i]
        o := 0.0
        if len(r.Indicators.Quote[0].Open) == len(r.Timestamp) {
            o = r.Indicators.Quote[0].Open[i]
        }
        if c > 0 {
            days = append(days, time.Date(ts.Year(), ts.Month(), ts.Day(), 0, 0, 0, 0, time.UTC))
            closes = append(closes, c)
            opens = append(opens, o)
        }
    }
    if len(days) == 0 {
        return 0, time.Time{}, ErrPriceNotFound
    }
    hs = histSeries{days: days, closes: closes, opens: opens, fetched: time.Now()}
    p.mu.Lock()
    p.hist[symbol] = hs
    p.mu.Unlock()
    return lookupHistClose(hs, date)
}

// GetPriceOnBasis returns a daily price with an explicit basis: "open" or "close".
func (p *YahooProvider) GetPriceOnBasis(symbol string, date time.Time, basis string) (float64, time.Time, error) {
    symbol = strings.ToUpper(strings.TrimSpace(symbol))
    if symbol == "" {
        return 0, time.Time{}, ErrPriceNotFound
    }
    date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
    p.mu.RLock()
    hs, ok := p.hist[symbol]
    if ok && time.Since(hs.fetched) < p.ttl && len(hs.days) > 0 {
        p.mu.RUnlock()
        if strings.EqualFold(basis, "open") {
            return lookupHistOpen(hs, date)
        }
        return lookupHistClose(hs, date)
    }
    p.mu.RUnlock()
    // Ensure cache is populated (reuse GetPriceOn path)
    _, _, err := p.GetPriceOn(symbol, date)
    if err != nil {
        return 0, time.Time{}, err
    }
    p.mu.RLock()
    hs = p.hist[symbol]
    p.mu.RUnlock()
    if strings.EqualFold(basis, "open") {
        return lookupHistOpen(hs, date)
    }
    return lookupHistClose(hs, date)
}

func lookupHistClose(hs histSeries, date time.Time) (float64, time.Time, error) {
    idx := -1
    for i := len(hs.days) - 1; i >= 0; i-- {
        if !hs.days[i].After(date) {
            idx = i
            break
        }
    }
    if idx < 0 {
        return 0, time.Time{}, ErrPriceNotFound
    }
    return hs.closes[idx], hs.days[idx], nil
}

func lookupHistOpen(hs histSeries, date time.Time) (float64, time.Time, error) {
    idx := -1
    for i := len(hs.days) - 1; i >= 0; i-- {
        if !hs.days[i].After(date) {
            idx = i
            break
        }
    }
    if idx < 0 {
        return 0, time.Time{}, ErrPriceNotFound
    }
    // If open is 0 (missing), fallback to close for that day
    o := 0.0
    if len(hs.opens) == len(hs.days) {
        o = hs.opens[idx]
    }
    if o > 0 {
        return o, hs.days[idx], nil
    }
    return hs.closes[idx], hs.days[idx], nil
}
