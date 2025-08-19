package main

import "time"

// ===== Domain =====

type TradeType string

const (
    TradeTypeBuy      TradeType = "buy"
    TradeTypeSell     TradeType = "sell"
    TradeTypeDividend TradeType = "dividend"
    TradeTypeCash     TradeType = "cash"
)

type Portfolio struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	BaseCCY   string    `json:"base_ccy"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Transaction struct {
	ID          string    `json:"id"`
	PortfolioID string    `json:"portfolio_id"`
	Symbol      string    `json:"symbol"` // e.g., AMZN, BHP.AX
	TradeType   TradeType `json:"trade_type"`
	Currency    string    `json:"currency"`
	Shares      float64   `json:"shares"`
	Price       float64   `json:"price"`
	Fee         float64   `json:"fee"`
	Date        time.Time `json:"date"`
	Total       float64   `json:"total"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
