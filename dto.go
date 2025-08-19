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

func (d portfolioDTO) validate() error {
	if strings.TrimSpace(d.Name) == "" {
		return errors.New("name is required")
	}
	return nil
}

func (d portfolioDTO) toDomain(now time.Time, idOpt ...string) (Portfolio, error) {
	if err := d.validate(); err != nil {
		return Portfolio{}, err
	}
	id := uuid.New().String()
	if len(idOpt) > 0 && idOpt[0] != "" {
		id = idOpt[0]
	}
	base := strings.ToUpper(strings.TrimSpace(d.BaseCCY))
	if base == "" {
		base = "TWD" // default ref currency => TWD
	}
	return Portfolio{
		ID:        id,
		Name:      strings.TrimSpace(d.Name),
		BaseCCY:   base,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

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
    case "buy":
        return TradeTypeBuy, nil
    case "sell":
        return TradeTypeSell, nil
    case "dividend":
        return TradeTypeDividend, nil
    case "cash":
        return TradeTypeCash, nil
    default:
        return "", fmt.Errorf("unsupported trade_type: %q (use buy|sell|dividend|cash)", tt)
    }
}

func (d transactionDTO) toDomain(now time.Time, portfolioID string, idOpt ...string) (Transaction, error) {
    t, err := time.ParseInLocation(payloadDateLayout, d.Date, time.Local)
	if err != nil {
		return Transaction{}, fmt.Errorf("invalid date %q (use YYYY/MM/DD): %w", d.Date, err)
	}

	id := uuid.New().String()
	if len(idOpt) > 0 && idOpt[0] != "" {
		id = idOpt[0]
	}
    tt, err := normalizeTradeType(d.TradeType)
    if err != nil {
        return Transaction{}, err
    }

    symbol := strings.ToUpper(strings.TrimSpace(d.Symbol))
    if symbol == "" && tt != TradeTypeCash {
        return Transaction{}, errors.New("symbol is required")
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
