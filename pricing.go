package main

import "time"

// PriceProvider returns the latest price for a symbol (in portfolio base ccy).
type PriceProvider interface {
	GetPrice(symbol string) (price float64, asOf time.Time, err error)
}
