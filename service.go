package main

import (
	"errors"
	"strings"
	"time"
)

/* ===================== Portfolio service ===================== */

type PortfolioService struct {
	repo PortfolioRepository
}

func NewPortfolioService(r PortfolioRepository) *PortfolioService {
	return &PortfolioService{repo: r}
}

func (s *PortfolioService) Create(dto portfolioDTO) (Portfolio, error) {
	now := time.Now()
	p, err := dto.toDomain(now)
	if err != nil {
		return Portfolio{}, err
	}
	return s.repo.Create(p)
}

func (s *PortfolioService) List() ([]Portfolio, error)       { return s.repo.List() }
func (s *PortfolioService) Get(id string) (Portfolio, error) { return s.repo.GetByID(id) }
func (s *PortfolioService) Delete(id string) error           { return s.repo.Delete(id) }

func (s *PortfolioService) Update(id string, dto portfolioDTO) (Portfolio, error) {
	now := time.Now()
	existing, err := s.repo.GetByID(id)
	if err != nil {
		return Portfolio{}, err
	}
	p, err := dto.toDomain(now, existing.ID)
	if err != nil {
		return Portfolio{}, err
	}
	p.CreatedAt = existing.CreatedAt
	return s.repo.Update(p)
}

/* ===================== Transaction service ===================== */

type TransactionService struct {
	repoTx TransactionRepository
	repoPf PortfolioRepository
	prices PriceProvider
}

func NewTransactionService(txRepo TransactionRepository, pfRepo PortfolioRepository, priceProvider PriceProvider) *TransactionService {
	return &TransactionService{repoTx: txRepo, repoPf: pfRepo, prices: priceProvider}
}

func (s *TransactionService) CreateOne(portfolioID string, dto transactionDTO) (Transaction, error) {
	if _, err := s.repoPf.GetByID(portfolioID); err != nil {
		return Transaction{}, ErrPortfolioNotFound
	}
	now := time.Now()
	tx, err := dto.toDomain(now, portfolioID)
	if err != nil {
		return Transaction{}, err
	}
	return s.repoTx.Create(portfolioID, tx)
}

func (s *TransactionService) CreateBatch(portfolioID string, dtos []transactionDTO) ([]Transaction, error) {
	if _, err := s.repoPf.GetByID(portfolioID); err != nil {
		return nil, ErrPortfolioNotFound
	}
	now := time.Now()
	txs := make([]Transaction, len(dtos))
	for i, d := range dtos {
		tx, err := d.toDomain(now, portfolioID)
		if err != nil {
			return nil, err
		}
		txs[i] = tx
	}
	return s.repoTx.CreateBatch(portfolioID, txs)
}

func (s *TransactionService) Get(portfolioID, id string) (Transaction, error) {
	return s.repoTx.GetByID(portfolioID, id)
}

func (s *TransactionService) List(portfolioID string, q ListFilter) ([]Transaction, error) {
	return s.repoTx.List(portfolioID, q)
}

func (s *TransactionService) Update(portfolioID, id string, dto transactionDTO) (Transaction, error) {
	existing, err := s.repoTx.GetByID(portfolioID, id)
	if err != nil {
		return Transaction{}, err
	}
	now := time.Now()
	tx, err := dto.toDomain(now, portfolioID, existing.ID)
	if err != nil {
		return Transaction{}, err
	}
	tx.CreatedAt = existing.CreatedAt
	return s.repoTx.Update(portfolioID, tx)
}

func (s *TransactionService) Delete(portfolioID, id string) error {
	return s.repoTx.Delete(portfolioID, id)
}

/* ===================== Allocations ===================== */

type AllocationItem struct {
	Symbol        string  `json:"symbol"`
	Shares        float64 `json:"shares"`
	Invested      float64 `json:"invested"`
	MarketValue   float64 `json:"market_value"`
	WeightPercent float64 `json:"weight_percent"`
}

type AllocationResponse struct {
	Basis            string           `json:"basis"` // "invested" | "market_value"
	TotalInvested    float64          `json:"total_invested,omitempty"`
	TotalMarketValue float64          `json:"total_market_value,omitempty"`
	AsOf             time.Time        `json:"as_of,omitempty"`
	Items            []AllocationItem `json:"items"`
}

// Per-portfolio
func (s *TransactionService) ComputeAllocations(portfolioID, basis string) (AllocationResponse, error) {
	if _, err := s.repoPf.GetByID(portfolioID); err != nil {
		return AllocationResponse{}, ErrPortfolioNotFound
	}
	all, err := s.repoTx.List(portfolioID, ListFilter{Limit: 0})
	if err != nil {
		return AllocationResponse{}, err
	}
	return s.computeAllocationsFromTxs(all, basis)
}

// Global (all portfolios)
func (s *TransactionService) ComputeAllocationsAll(basis string) (AllocationResponse, error) {
	pfs, err := s.repoPf.List()
	if err != nil {
		return AllocationResponse{}, err
	}
	var all []Transaction
	for _, pf := range pfs {
		txs, err := s.repoTx.List(pf.ID, ListFilter{Limit: 0})
		if err != nil {
			return AllocationResponse{}, err
		}
		all = append(all, txs...)
	}
	return s.computeAllocationsFromTxs(all, basis)
}

func (s *TransactionService) computeAllocationsFromTxs(all []Transaction, basis string) (AllocationResponse, error) {
	type agg struct {
		shares   float64
		invested float64
	}
	bucket := map[string]*agg{}

	for _, tx := range all {
		a := bucket[tx.Symbol]
		if a == nil {
			a = &agg{}
			bucket[tx.Symbol] = a
		}
		switch tx.TradeType {
		case TradeTypeBuy:
			a.shares += tx.Shares
			if tx.Total < 0 {
				a.invested += -tx.Total
			} else {
				a.invested += tx.Total
			}
		case TradeTypeSell:
			a.shares -= tx.Shares
		case TradeTypeDividend:
			// ignore
		}
	}

	items := make([]AllocationItem, 0, len(bucket))
	switch strings.ToLower(basis) {
	case "", "invested":
		var totalInv float64
		for sym, a := range bucket {
			items = append(items, AllocationItem{Symbol: sym, Shares: a.shares, Invested: a.invested})
			totalInv += a.invested
		}
		for i := range items {
			if totalInv > 0 {
				items[i].WeightPercent = (items[i].Invested / totalInv) * 100.0
			}
		}
		return AllocationResponse{
			Basis:         "invested",
			TotalInvested: totalInv,
			Items:         items,
		}, nil

	case "market_value":
		if s.prices == nil {
			return AllocationResponse{}, errors.New("no PriceProvider configured for market_value basis")
		}
		var totalMV float64
		var asOf time.Time
		for sym, a := range bucket {
			if a.shares <= 0 {
				continue
			}
			price, ts, err := s.prices.GetPrice(sym)
			if err != nil {
				continue // skip symbols we can't price
			}
			mv := a.shares * price
			items = append(items, AllocationItem{
				Symbol:      sym,
				Shares:      a.shares,
				Invested:    a.invested,
				MarketValue: mv,
			})
			totalMV += mv
			if ts.After(asOf) {
				asOf = ts
			}
		}
		for i := range items {
			if totalMV > 0 {
				items[i].WeightPercent = (items[i].MarketValue / totalMV) * 100.0
			}
		}
		return AllocationResponse{
			Basis:            "market_value",
			TotalMarketValue: totalMV,
			AsOf:             asOf,
			Items:            items,
		}, nil

	default:
		return AllocationResponse{}, errors.New(`unsupported basis (use "invested" or "market_value")`)
	}
}

/* ===================== Global summary ===================== */

type PositionSummary struct {
	Symbol                 string  `json:"symbol"`
	Shares                 float64 `json:"shares"`
	Invested               float64 `json:"invested"`
	MarketValue            float64 `json:"market_value"`
	UnrealizedPL           float64 `json:"unrealized_pl"`
	UnrealizedPLPercent    float64 `json:"unrealized_pl_percent"`
	WeightPercentByMV      float64 `json:"weight_percent_by_market_value"`
}

type SummaryResponse struct {
	AsOf                  time.Time         `json:"as_of"`
	TotalInvested         float64           `json:"total_invested"`
	TotalMarketValue      float64           `json:"total_market_value"`
	TotalUnrealizedPL     float64           `json:"total_unrealized_pl"`
	TotalUnrealizedPLPerc float64           `json:"total_unrealized_pl_percent"`
	Positions             []PositionSummary `json:"positions"`
}

// Overall (all portfolios). P/L here is UNREALIZED = MV − invested.
// "Invested" = sum ABS(purchase totals); sells don't reduce invested.
func (s *TransactionService) ComputeSummaryAll() (SummaryResponse, error) {
	if s.prices == nil {
		return SummaryResponse{}, errors.New("no PriceProvider configured (required for summary)")
	}
	pfs, err := s.repoPf.List()
	if err != nil {
		return SummaryResponse{}, err
	}

	type agg struct {
		shares   float64
		invested float64
	}
	bucket := map[string]*agg{}

	for _, pf := range pfs {
		txs, err := s.repoTx.List(pf.ID, ListFilter{Limit: 0})
		if err != nil {
			return SummaryResponse{}, err
		}
		for _, tx := range txs {
			a := bucket[tx.Symbol]
			if a == nil {
				a = &agg{}
				bucket[tx.Symbol] = a
			}
			switch tx.TradeType {
			case TradeTypeBuy:
				a.shares += tx.Shares
				if tx.Total < 0 {
					a.invested += -tx.Total
				} else {
					a.invested += tx.Total
				}
			case TradeTypeSell:
				a.shares -= tx.Shares
			case TradeTypeDividend:
				// ignore
			}
		}
	}

	out := SummaryResponse{}
	var totalMV, totalInv float64
	var asOf time.Time

	positions := make([]PositionSummary, 0, len(bucket))
	for sym, a := range bucket {
		if a.shares <= 0 && a.invested == 0 {
			continue
		}
		price, ts, err := s.prices.GetPrice(sym)
		if err != nil {
			continue
		}
		mv := a.shares * price
		pl := mv - a.invested
		plPct := 0.0
		if a.invested > 0 {
			plPct = (pl / a.invested) * 100.0
		}
		positions = append(positions, PositionSummary{
			Symbol:              sym,
			Shares:              a.shares,
			Invested:            a.invested,
			MarketValue:         mv,
			UnrealizedPL:        pl,
			UnrealizedPLPercent: plPct,
		})
		totalMV += mv
		totalInv += a.invested
		if ts.After(asOf) {
			asOf = ts
		}
	}

	for i := range positions {
		if totalMV > 0 {
			positions[i].WeightPercentByMV = (positions[i].MarketValue / totalMV) * 100.0
		}
	}

	out.AsOf = asOf
	out.TotalInvested = totalInv
	out.TotalMarketValue = totalMV
	out.TotalUnrealizedPL = totalMV - totalInv
	if totalInv > 0 {
		out.TotalUnrealizedPLPerc = ((totalMV - totalInv) / totalInv) * 100.0
	}
	out.Positions = positions
	return out, nil
}
