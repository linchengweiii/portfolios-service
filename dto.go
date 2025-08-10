package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ===== DTOs =====

type portfolioDTO struct {
	Name    string `json:"name"`
	BaseCCY string `json:"base_ccy,omitempty"`
}

func (d portfolioDTO) toDomain(now time.Time, idOpt ...string) (Portfolio, error) {
	if strings.TrimSpace(d.Name) == "" {
		return Portfolio{}, errors.New("name is required")
	}
	id := ""
	if len(idOpt) > 0 {
		id = idOpt[0]
	}
	if id == "" {
		id = uuid.NewString()
	}
	return Portfolio{
		ID:        id,
		Name:      strings.TrimSpace(d.Name),
		BaseCCY:   strings.ToUpper(strings.TrimSpace(d.BaseCCY)),
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// Transaction DTO (date is "YYYY/MM/DD"). Only "symbol".
type transactionDTO struct {
	Symbol    string    `json:"symbol"`
	TradeType TradeType `json:"trade_type"`
	Currency  string    `json:"currency"`
	Shares    float64   `json:"shares"`
	Price     float64   `json:"price"`
	Fee       float64   `json:"fee"`
	Date      string    `json:"date"` // "2025/08/06"
	Total     float64   `json:"total"`
}

const payloadDateLayout = "2006/01/02"

func normalizeTradeType(tt TradeType) (TradeType, error) {
	switch strings.ToLower(string(tt)) {
	case "purchase":
		return TradeTypePurchase, nil
	case "sell":
		return TradeTypeSell, nil
	case "dividend":
		return TradeTypeDividend, nil
	default:
		return "", fmt.Errorf("unsupported trade_type: %q (use purchase|sell|dividend)", tt)
	}
}

func (d transactionDTO) toDomain(now time.Time, portfolioID string, idOpt ...string) (Transaction, error) {
	t, err := time.ParseInLocation(payloadDateLayout, d.Date, time.Local)
	if err != nil {
		return Transaction{}, fmt.Errorf("invalid date %q (use YYYY/MM/DD): %w", d.Date, err)
	}
	symbol := strings.ToUpper(strings.TrimSpace(d.Symbol))
	if symbol == "" || d.Currency == "" || d.TradeType == "" {
		return Transaction{}, errors.New("symbol, currency, trade_type are required")
	}
	if d.Shares < 0 || d.Price < 0 || d.Fee < 0 {
		return Transaction{}, errors.New("shares, price, and fee must be >= 0")
	}
	tt, err := normalizeTradeType(d.TradeType)
	if err != nil {
		return Transaction{}, err
	}
	id := ""
	if len(idOpt) > 0 {
		id = idOpt[0]
	}
	if id == "" {
		id = uuid.NewString()
	}
	return Transaction{
		ID:          id,
		PortfolioID: portfolioID,
		Symbol:      symbol,
		TradeType:   tt,
		Currency:    d.Currency,
		Shares:      d.Shares,
		Price:       d.Price,
		Fee:         d.Fee,
		Date:        t,
		Total:       d.Total,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}
