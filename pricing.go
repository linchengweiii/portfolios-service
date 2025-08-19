package main

import "time"

// PriceProvider returns the latest price for a symbol (in the quote's own currency).
type PriceProvider interface {
    GetPrice(symbol string) (price float64, asOf time.Time, err error)
}

// CurrencyExchanger converts money from one currency into another.
type CurrencyExchanger interface {
    // Rate returns how many 'to' units per 1 'from' unit. (amount_in_to = amount_in_from * rate)
    Rate(from, to string) (rate float64, asOf time.Time, err error)
}

// HistoryProvider optionally provides daily historical prices.
// Implementations should return the last available CLOSE price at or before the given date.
type HistoryProvider interface {
    GetPriceOn(symbol string, date time.Time) (price float64, asOf time.Time, err error)
}
