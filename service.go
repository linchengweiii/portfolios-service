package main

import (
    "errors"
    "regexp"
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
    repoTx    TransactionRepository
    repoPf    PortfolioRepository
    prices    PriceProvider
    exchanger CurrencyExchanger
    refCCY    string
}

func NewTransactionService(txRepo TransactionRepository, pfRepo PortfolioRepository, priceProvider PriceProvider, exchanger CurrencyExchanger, refCCY string) *TransactionService {
	if refCCY == "" {
		refCCY = "TWD"
	}
    return &TransactionService{
        repoTx:    txRepo,
        repoPf:    pfRepo,
        prices:    priceProvider,
        exchanger: exchanger,
        refCCY:    strings.ToUpper(refCCY),
    }
}

// WithRef returns a shallow copy of the service using the provided
// reference currency for calculations. Only TWD and USD are accepted
// for now; anything else falls back to TWD.
func (s *TransactionService) WithRef(ref string) *TransactionService {
    r := strings.ToUpper(strings.TrimSpace(ref))
    switch r {
    case "USD", "TWD":
        // ok
    default:
        if s != nil && s.refCCY != "" && (s.refCCY == "USD" || s.refCCY == "TWD") {
            r = s.refCCY
        } else {
            r = "TWD"
        }
    }
    cp := *s
    cp.refCCY = r
    return &cp
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

func (s *TransactionService) rate(from string) float64 {
	if s.exchanger == nil || strings.EqualFold(from, s.refCCY) || strings.TrimSpace(from) == "" {
		return 1.0
	}
	r, _, err := s.exchanger.Rate(from, s.refCCY)
	if err != nil || r <= 0 {
		return 1.0 // graceful fallback
	}
	return r
}

// Detect option symbols and return contract multiplier.
// For standard US equity options, Yahoo symbols look like: AAPL240118C00150000
// Pattern: TICKER(1-6 letters) + YYMMDD + C|P + 8-digit strike.
var reOptionSymbol = regexp.MustCompile(`^[A-Z]{1,6}\d{6}[CP]\d{8}$`)

func multiplierForSymbol(sym string) float64 {
    s := strings.ToUpper(strings.TrimSpace(sym))
    if reOptionSymbol.MatchString(s) {
        return 100.0
    }
    return 1.0
}

// sameYMD returns true if two timestamps share the same UTC year-month-day.
func sameYMD(a, b time.Time) bool {
    a = a.UTC()
    b = b.UTC()
    return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

/* ===================== Allocations ===================== */

type AllocationItem struct {
    Symbol        string  `json:"symbol"`
    Shares        float64 `json:"shares"`
    Invested      float64 `json:"invested"`
    MarketValue   float64 `json:"market_value"`
    WeightPercent float64 `json:"weight_percent"`
    // Optional daily P/L stats when a history-capable price provider is available
    DailyPL        float64 `json:"daily_pl,omitempty"`
    DailyPLPercent float64 `json:"daily_pl_percent,omitempty"`
    // Yesterday's market value used as the denominator for DailyPLPercent
    DailyPrevMarketValue float64 `json:"daily_prev_market_value,omitempty"`
}

type AllocationResponse struct {
	Basis            string           `json:"basis"` // "invested" | "market_value"
	TotalInvested    float64          `json:"total_invested,omitempty"`
	TotalMarketValue float64          `json:"total_market_value,omitempty"`
	AsOf             time.Time        `json:"as_of,omitempty"`
	RefCurrency      string           `json:"ref_currency"`
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
        invested float64 // cost of remaining shares in ref currency (after sells reduce by avg cost)
        currency string  // last seen tx currency for the symbol
    }
    bucket := map[string]*agg{}

    // Process in chronological order so average-cost reductions on sell are correct
    insertionSort(all, func(a, b Transaction) bool { return a.Date.Before(b.Date) })

    for _, tx := range all {
        switch tx.TradeType {
        case TradeTypeBuy, TradeTypeSell, TradeTypeDividend:
            a := bucket[tx.Symbol]
            if a == nil {
                a = &agg{}
                bucket[tx.Symbol] = a
            }
            if tx.Currency != "" {
                a.currency = strings.ToUpper(tx.Currency)
            }
            switch tx.TradeType {
            case TradeTypeBuy:
                a.shares += tx.Shares
                amt := tx.Total
                if amt < 0 {
                    amt = -amt
                }
                a.invested += amt * s.rate(tx.Currency)
            case TradeTypeSell:
                // Reduce invested by average cost per share for the shares sold
                if a.shares > 0 {
                    avgCost := 0.0
                    if a.shares > 0 {
                        avgCost = a.invested / a.shares
                    }
                    sellShares := tx.Shares
                    if sellShares > a.shares {
                        sellShares = a.shares
                    }
                    a.invested -= avgCost * sellShares
                    if a.invested < 0 {
                        a.invested = 0
                    }
                }
                a.shares -= tx.Shares
            case TradeTypeDividend:
                // no change to invested/shares
            }
        }
    }

	items := make([]AllocationItem, 0, len(bucket))
	switch strings.ToLower(basis) {
	case "", "invested":
		var totalInv float64
		for sym, a := range bucket {
			if a.shares <= 0 && a.invested == 0 {
				continue
			}
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
			RefCurrency:   s.refCCY,
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
            mult := multiplierForSymbol(sym)
            mv := a.shares * price * mult * s.rate(a.currency)

            it := AllocationItem{
                Symbol:      sym,
                Shares:      a.shares,
                Invested:    a.invested,
                MarketValue: mv,
            }

            // Populate per-item daily P/L if historical prices are available
            if hp, ok := s.prices.(HistoryProvider); ok {
                today := time.Now().UTC()
                if cur, asOfDay, err1 := hp.GetPriceOn(sym, today); err1 == nil && cur > 0 {
                    if prev, _, err2 := hp.GetPriceOn(sym, asOfDay.AddDate(0, 0, -1)); err2 == nil && prev > 0 {
                        rate := s.rate(a.currency)
                        mult := multiplierForSymbol(sym)
                        dailyPL := a.shares * (cur - prev) * mult * rate
                        // Denominator is yesterday's MV for the symbol
                        prevMV := a.shares * prev * mult * rate
                        it.DailyPL = dailyPL
                        it.DailyPrevMarketValue = prevMV
                        if prevMV > 0 {
                            it.DailyPLPercent = (dailyPL / prevMV) * 100.0
                        }
                    }
                }
            }

            items = append(items, it)
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
			RefCurrency:      s.refCCY,
			Items:            items,
		}, nil

	default:
		return AllocationResponse{}, errors.New(`unsupported basis (use "invested" or "market_value")`)
	}
}

/* ===================== Global summary ===================== */

type PositionSummary struct {
	Symbol              string  `json:"symbol"`
	Shares              float64 `json:"shares"`
	Invested            float64 `json:"invested"`
	MarketValue         float64 `json:"market_value"`
	UnrealizedPL        float64 `json:"unrealized_pl"`
	UnrealizedPLPercent float64 `json:"unrealized_pl_percent"`
	WeightPercentByMV   float64 `json:"weight_percent_by_market_value"`
}

type SummaryResponse struct {
    AsOf                  time.Time         `json:"as_of"`
    RefCurrency           string            `json:"ref_currency"`
    TotalInvested         float64           `json:"total_invested"`
    TotalMarketValue      float64           `json:"total_market_value"`
    TotalUnrealizedPL     float64           `json:"total_unrealized_pl"`
    TotalUnrealizedPLPerc float64           `json:"total_unrealized_pl_percent"`
    TotalUnrealizedPLPercCurrent float64    `json:"total_unrealized_pl_percent_current,omitempty"`
    DailyPL               float64           `json:"daily_pl,omitempty"`
    DailyPLPercent        float64           `json:"daily_pl_percent,omitempty"`
    Balance               float64           `json:"balance"`
    CashDeposits          float64           `json:"cash_deposits,omitempty"`
    CashWithdrawals       float64           `json:"cash_withdrawals,omitempty"`
    InferredDeposits      float64           `json:"inferred_deposits,omitempty"`
    EffectiveCashIn       float64           `json:"effective_cash_in,omitempty"`
    EffectiveCashInPeak   float64           `json:"effective_cash_in_peak,omitempty"`
    Positions             []PositionSummary `json:"positions"`
}

// Overall (all portfolios). P/L here is UNREALIZED = MV − invested.
// "Invested" = sum ABS(purchase totals) converted to refCCY; sells don't reduce invested.
// Also: drop positions with zero shares (your request).
func (s *TransactionService) ComputeSummaryAll() (SummaryResponse, error) {
    if s.prices == nil {
        return SummaryResponse{}, errors.New("no PriceProvider configured (required for summary)")
    }
    pfs, err := s.repoPf.List()
    if err != nil {
        return SummaryResponse{}, err
    }
    // Build positions across all portfolios and compute per-portfolio balances (assuming no withdrawals)
    type agg struct {
        shares   float64
        invested float64
        currency string
    }
    bucket := map[string]*agg{}
    var sumBalance float64
    var sumDeposits float64
    var sumWithdrawals float64
    var sumInferred float64
    var sumEffectiveIn float64
    var sumPeakIn float64
    for _, pf := range pfs {
        txs, err := s.repoTx.List(pf.ID, ListFilter{Limit: 0})
        if err != nil {
            return SummaryResponse{}, err
        }
        // accumulate per-portfolio cash stats
        cs := s.computeCashStats(txs)
        sumBalance += cs.balance
        sumDeposits += cs.deposits
        sumWithdrawals += cs.withdrawals
        sumInferred += cs.inferred
        sumEffectiveIn += cs.effectiveIn
        sumPeakIn += cs.peakContrib
        // accumulate positions using average cost
        insertionSort(txs, func(a, b Transaction) bool { return a.Date.Before(b.Date) })
        for _, tx := range txs {
            switch tx.TradeType {
            case TradeTypeBuy, TradeTypeSell, TradeTypeDividend:
                a := bucket[tx.Symbol]
                if a == nil {
                    a = &agg{}
                    bucket[tx.Symbol] = a
                }
                if tx.Currency != "" {
                    a.currency = strings.ToUpper(tx.Currency)
                }
                switch tx.TradeType {
                case TradeTypeBuy:
                    a.shares += tx.Shares
                    amt := tx.Total
                    if amt < 0 {
                        amt = -amt
                    }
                    a.invested += amt * s.rate(tx.Currency)
                case TradeTypeSell:
                    if a.shares > 0 {
                        avgCost := 0.0
                        if a.shares > 0 {
                            avgCost = a.invested / a.shares
                        }
                        sellShares := tx.Shares
                        if sellShares > a.shares {
                            sellShares = a.shares
                        }
                        a.invested -= avgCost * sellShares
                        if a.invested < 0 {
                            a.invested = 0
                        }
                    }
                    a.shares -= tx.Shares
                case TradeTypeDividend:
                    // no effect on invested/shares
                }
            }
        }
    }

    out := SummaryResponse{RefCurrency: s.refCCY}
    var totalMV, totalInv float64
    var asOf time.Time
    var dailyPL float64
    var prevMV float64
    positions := make([]PositionSummary, 0, len(bucket))
    for sym, a := range bucket {
        if a.shares <= 0 {
            continue
        }
        price, ts, err := s.prices.GetPrice(sym)
        if err != nil {
            continue
        }
        mult := multiplierForSymbol(sym)
        mv := a.shares * price * mult * s.rate(a.currency)
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

        // Daily P/L = shares * (close_today - close_prev) converted to ref currency
        if hp, ok := s.prices.(HistoryProvider); ok {
            today := time.Now().UTC()
            cur, asOfDay, err1 := hp.GetPriceOn(sym, today)
            if err1 == nil && cur > 0 {
                prev, _, err2 := hp.GetPriceOn(sym, asOfDay.AddDate(0, 0, -1))
                if err2 == nil && prev > 0 {
                    rate := s.rate(a.currency)
                    mult := multiplierForSymbol(sym)
                    dailyPL += a.shares * (cur - prev) * mult * rate
                    prevMV += a.shares * prev * mult * rate
                }
            }
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
    // Cash-based P/L = Equity - EffectiveCashIn
    effectiveCashIn := sumEffectiveIn
    peakCashIn := sumPeakIn
    equity := totalMV + sumBalance
    out.TotalUnrealizedPL = equity - effectiveCashIn
    out.DailyPL = dailyPL
    if prevMV > 0 {
        out.DailyPLPercent = (dailyPL / prevMV) * 100.0
    }
    out.Balance = sumBalance
    out.CashDeposits = sumDeposits
    out.CashWithdrawals = sumWithdrawals
    out.InferredDeposits = sumInferred
    out.EffectiveCashIn = effectiveCashIn
    out.EffectiveCashInPeak = peakCashIn
    if peakCashIn > 0 {
        out.TotalUnrealizedPLPerc = (out.TotalUnrealizedPL / peakCashIn) * 100.0
    }
    if effectiveCashIn > 0 {
        out.TotalUnrealizedPLPercCurrent = (out.TotalUnrealizedPL / effectiveCashIn) * 100.0
    }
    out.Positions = positions
    return out, nil
}

// Per-portfolio summary
func (s *TransactionService) ComputeSummary(portfolioID string) (SummaryResponse, error) {
    if s.prices == nil {
        return SummaryResponse{}, errors.New("no PriceProvider configured (required for summary)")
    }
    if _, err := s.repoPf.GetByID(portfolioID); err != nil {
        return SummaryResponse{}, ErrPortfolioNotFound
    }
    txs, err := s.repoTx.List(portfolioID, ListFilter{Limit: 0})
    if err != nil {
        return SummaryResponse{}, err
    }
    return s.computeSummaryFromTxs(txs)
}

// Shared summary computation from a list of transactions.
func (s *TransactionService) computeSummaryFromTxs(allTx []Transaction) (SummaryResponse, error) {
    type agg struct {
        shares   float64
        invested float64 // cost of remaining shares in ref currency (after sells reduce by avg cost)
        currency string  // last seen tx currency for the symbol
    }
    bucket := map[string]*agg{}

    // Sort by date for correct average cost handling on sells
    insertionSort(allTx, func(a, b Transaction) bool { return a.Date.Before(b.Date) })

    for _, tx := range allTx {
        // Position aggregation (ignore cash)
        switch tx.TradeType {
        case TradeTypeBuy, TradeTypeSell, TradeTypeDividend:
            a := bucket[tx.Symbol]
            if a == nil {
                a = &agg{}
                bucket[tx.Symbol] = a
            }
            if tx.Currency != "" {
                a.currency = strings.ToUpper(tx.Currency)
            }
            switch tx.TradeType {
            case TradeTypeBuy:
                a.shares += tx.Shares
                amt := tx.Total
                if amt < 0 {
                    amt = -amt
                }
                a.invested += amt * s.rate(tx.Currency)
            case TradeTypeSell:
                // Reduce invested by average cost per share for the shares sold
                if a.shares > 0 {
                    avgCost := 0.0
                    if a.shares > 0 {
                        avgCost = a.invested / a.shares
                    }
                    sellShares := tx.Shares
                    if sellShares > a.shares {
                        sellShares = a.shares
                    }
                    a.invested -= avgCost * sellShares
                    if a.invested < 0 {
                        a.invested = 0
                    }
                }
                a.shares -= tx.Shares
            case TradeTypeDividend:
                // no change to invested/shares
            }
        }
    }

    out := SummaryResponse{RefCurrency: s.refCCY}
    var totalMV, totalInv float64
    var asOf time.Time
    var dailyPL float64
    var prevMV float64
    positions := make([]PositionSummary, 0, len(bucket))
    for sym, a := range bucket {
        if a.shares <= 0 {
            continue
        }
        price, ts, err := s.prices.GetPrice(sym)
        if err != nil {
            continue
        }
        mult := multiplierForSymbol(sym)
        mv := a.shares * price * mult * s.rate(a.currency)
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

        // Daily P/L = shares * (close_today - close_prev) converted to ref currency
        if hp, ok := s.prices.(HistoryProvider); ok {
            today := time.Now().UTC()
            cur, asOfDay, err1 := hp.GetPriceOn(sym, today)
            if err1 == nil && cur > 0 {
                prev, _, err2 := hp.GetPriceOn(sym, asOfDay.AddDate(0, 0, -1))
                if err2 == nil && prev > 0 {
                    rate := s.rate(a.currency)
                    mult := multiplierForSymbol(sym)
                    dailyPL += a.shares * (cur - prev) * mult * rate
                    prevMV += a.shares * prev * mult * rate
                }
            }
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

    // Cash-based stats (deposits/withdrawals/inferred/balance)
    cs := s.computeCashStats(allTx)
    out.Balance = cs.balance
    out.CashDeposits = cs.deposits
    out.CashWithdrawals = cs.withdrawals
    out.InferredDeposits = cs.inferred
    out.EffectiveCashIn = cs.effectiveIn
    out.EffectiveCashInPeak = cs.peakContrib
    // Cash-based P/L = Equity - EffectiveCashIn (current-basis).
    effectiveCashIn := cs.effectiveIn
    equity := out.TotalMarketValue + out.Balance
    out.TotalUnrealizedPL = equity - effectiveCashIn
    out.DailyPL = dailyPL
    if prevMV > 0 {
        out.DailyPLPercent = (dailyPL / prevMV) * 100.0
    }
    if cs.peakContrib > 0 {
        out.TotalUnrealizedPLPerc = (out.TotalUnrealizedPL / cs.peakContrib) * 100.0
    }
    if effectiveCashIn > 0 {
        out.TotalUnrealizedPLPercCurrent = (out.TotalUnrealizedPL / effectiveCashIn) * 100.0
    }
    out.Positions = positions
    return out, nil
}

// inferBalance computes the ending balance assuming no withdrawals, and
// injecting the minimal deposits needed so the running balance never goes below zero.
func (s *TransactionService) inferBalance(txs []Transaction) float64 {
    if len(txs) == 0 {
        return 0
    }
    // Copy and sort by date; for same date, place inflows before outflows
    xs := make([]Transaction, len(txs))
    copy(xs, txs)
    insertionSort(xs, func(a, b Transaction) bool {
        if a.Date.Before(b.Date) {
            return true
        }
        if a.Date.After(b.Date) {
            return false
        }
        // Same timestamp: inflows before outflows to maximize non-negative balance
        deltaA := func(tx Transaction) float64 {
            switch tx.TradeType {
            case TradeTypeBuy:
                amt := tx.Total
                if amt < 0 {
                    amt = -amt
                }
                return -amt * s.rate(tx.Currency)
            case TradeTypeSell:
                amt := tx.Total
                if amt < 0 {
                    amt = -amt
                }
                return +amt * s.rate(tx.Currency)
            case TradeTypeDividend:
                amt := tx.Total
                if amt < 0 {
                    amt = -amt
                }
                return +amt * s.rate(tx.Currency)
            case TradeTypeCash:
                return tx.Total * s.rate(tx.Currency)
            default:
                return 0
            }
        }
        da := deltaA(a)
        db := deltaA(b)
        if da == db {
            // deterministic tie-breaker
            return a.ID < b.ID
        }
        // Want inflows (positive delta) before outflows (negative delta)
        return da > db
    })

    var sum float64
    var prefix float64
    var minPrefix float64
    for _, tx := range xs {
        var delta float64
        switch tx.TradeType {
        case TradeTypeBuy:
            amt := tx.Total
            if amt < 0 {
                amt = -amt
            }
            delta = -amt * s.rate(tx.Currency)
        case TradeTypeSell:
            amt := tx.Total
            if amt < 0 {
                amt = -amt
            }
            delta = +amt * s.rate(tx.Currency)
        case TradeTypeDividend:
            amt := tx.Total
            if amt < 0 {
                amt = -amt
            }
            delta = +amt * s.rate(tx.Currency)
        case TradeTypeCash:
            // Deposits positive, withdrawals negative as provided
            delta = tx.Total * s.rate(tx.Currency)
        default:
            delta = 0
        }
        sum += delta
        prefix += delta
        if prefix < minPrefix {
            minPrefix = prefix
        }
    }
    inferredDeposit := 0.0
    if minPrefix < 0 {
        inferredDeposit = -minPrefix
    }
    return inferredDeposit + sum
}

type cashStats struct {
    deposits    float64
    withdrawals float64
    inferred    float64
    balance     float64
    effectiveIn float64
    peakContrib float64
    inferredEvents   []cashEvent
    depositEvents    []cashEvent
    withdrawalEvents []cashEvent
}

// computeCashStats sorts the transactions by date (inflows before outflows within the same date),
// computes deposits, withdrawals, minimal inferred deposits to avoid negative balance, and ending balance.
func (s *TransactionService) computeCashStats(txs []Transaction) cashStats {
    if len(txs) == 0 {
        return cashStats{}
    }
    xs := make([]Transaction, len(txs))
    copy(xs, txs)
    // Sort with inflows before outflows at equal timestamps
    insertionSort(xs, func(a, b Transaction) bool {
        if a.Date.Before(b.Date) {
            return true
        }
        if a.Date.After(b.Date) {
            return false
        }
        deltaA := func(tx Transaction) float64 {
            switch tx.TradeType {
            case TradeTypeBuy:
                amt := tx.Total
                if amt < 0 {
                    amt = -amt
                }
                return -amt * s.rate(tx.Currency)
            case TradeTypeSell:
                amt := tx.Total
                if amt < 0 {
                    amt = -amt
                }
                return +amt * s.rate(tx.Currency)
            case TradeTypeDividend:
                amt := tx.Total
                if amt < 0 {
                    amt = -amt
                }
                return +amt * s.rate(tx.Currency)
            case TradeTypeCash:
                return tx.Total * s.rate(tx.Currency)
            default:
                return 0
            }
        }
        da := deltaA(a)
        db := deltaA(b)
        if da == db {
            return a.ID < b.ID
        }
        return da > db
    })

    var sum float64            // running cash balance
    var prefix float64         // same as sum, kept for clarity
    var minPrefix float64
    var deposits float64
    var withdrawals float64
    var contribPrefix float64  // running net contributions (deposits - withdrawals + inferred)
    var peakContrib float64
    var inferredTotal float64
    var inferredEvents []cashEvent
    var depositEvents []cashEvent
    var withdrawalEvents []cashEvent
    for _, tx := range xs {
        var delta float64
        switch tx.TradeType {
        case TradeTypeBuy:
            amt := tx.Total
            if amt < 0 {
                amt = -amt
            }
            delta = -amt * s.rate(tx.Currency)
        case TradeTypeSell:
            amt := tx.Total
            if amt < 0 {
                amt = -amt
            }
            delta = +amt * s.rate(tx.Currency)
        case TradeTypeDividend:
            amt := tx.Total
            if amt < 0 {
                amt = -amt
            }
            delta = +amt * s.rate(tx.Currency)
        case TradeTypeCash:
            v := tx.Total * s.rate(tx.Currency)
            delta = v
            if v >= 0 {
                deposits += v
                contribPrefix += v
                depositEvents = append(depositEvents, cashEvent{when: tx.Date, amount: v})
            } else {
                w := -v
                withdrawals += w
                contribPrefix -= w
                if contribPrefix < 0 {
                    contribPrefix = 0 // don't let net contributions go negative
                }
                withdrawalEvents = append(withdrawalEvents, cashEvent{when: tx.Date, amount: w})
            }
        }
        // Before applying delta, if it would take balance negative, inject minimal inferred deposit
        if prefix+delta < 0 {
            need := -(prefix + delta)
            inferredTotal += need
            contribPrefix += need
            prefix += need
            sum += need
            inferredEvents = append(inferredEvents, cashEvent{when: tx.Date, amount: need})
        }
        sum += delta
        prefix += delta
        if prefix < minPrefix {
            minPrefix = prefix
        }
        if contribPrefix > peakContrib {
            peakContrib = contribPrefix
        }
    }
    inferred := inferredTotal
    return cashStats{
        deposits:    deposits,
        withdrawals: withdrawals,
        inferred:    inferred,
        balance:     sum, // inferred was already injected during the run
        effectiveIn: deposits - withdrawals + inferred,
        peakContrib: peakContrib,
        inferredEvents:   inferredEvents,
        depositEvents:    depositEvents,
        withdrawalEvents: withdrawalEvents,
    }
}

type cashEvent struct {
    when   time.Time
    amount float64 // always positive magnitude in ref currency
}

// Backtest result comparing alternate asset vs current portfolio
type BacktestResponse struct {
    Symbol          string    `json:"symbol"`
    AsOf            time.Time `json:"as_of"`
    RefCurrency     string    `json:"ref_currency"`
    AltPL           float64   `json:"alt_pl"`
    AltPLPercent    float64   `json:"alt_pl_percent"`
    // AltMaxDropPercent is the maximum percentage drop from a prior
    // peak of the simulated alternate equity curve (in ref currency).
    // Expressed as a negative percentage, e.g., -25.3 for a 25.3% drop.
    AltMaxDropPercent float64 `json:"alt_max_drop_percent"`
    CurrentPL       float64   `json:"current_pl"`
    CurrentPLPercent float64  `json:"current_pl_percent"`
    // CurrentMaxDropPercent is the maximum percentage drop from a prior
    // peak of the actual portfolio equity curve (MV+cash in ref currency),
    // sampled at transaction dates and as-of. Negative percentage.
    CurrentMaxDropPercent float64 `json:"current_max_drop_percent"`
    Debug           *BacktestDebug `json:"debug,omitempty"`
}

type BacktestEventDebug struct {
    When         time.Time `json:"when"`
    Kind         string    `json:"kind"` // deposit | withdrawal
    AmountRef    float64   `json:"amount_ref"`
    Price        float64   `json:"price"`
    PriceAsOf    time.Time `json:"price_as_of"`
    SharesDelta  float64   `json:"shares_delta"`
    SharesTotal  float64   `json:"shares_total"`
    EquityRef    float64   `json:"equity_ref_after"`
}

type BacktestDebug struct {
    Events []BacktestEventDebug `json:"events"`
}

// Per-portfolio backtest
func (s *TransactionService) ComputeBacktest(portfolioID, symbol, symbolCCY, priceBasis string, debug bool) (BacktestResponse, error) {
    if _, err := s.repoPf.GetByID(portfolioID); err != nil {
        return BacktestResponse{}, ErrPortfolioNotFound
    }
    txs, err := s.repoTx.List(portfolioID, ListFilter{Limit: 0})
    if err != nil {
        return BacktestResponse{}, err
    }
    return s.computeBacktestFromTxs(txs, symbol, symbolCCY, priceBasis, debug)
}

// Global backtest
func (s *TransactionService) ComputeBacktestAll(symbol, symbolCCY, priceBasis string, debug bool) (BacktestResponse, error) {
    pfs, err := s.repoPf.List()
    if err != nil {
        return BacktestResponse{}, err
    }
    var all []Transaction
    for _, pf := range pfs {
        txs, err := s.repoTx.List(pf.ID, ListFilter{Limit: 0})
        if err != nil {
            return BacktestResponse{}, err
        }
        all = append(all, txs...)
    }
    return s.computeBacktestFromTxs(all, symbol, symbolCCY, priceBasis, debug)
}

func (s *TransactionService) computeBacktestFromTxs(allTx []Transaction, symbol, symbolCCY, priceBasis string, debug bool) (BacktestResponse, error) {
    if s.prices == nil {
        return BacktestResponse{}, errors.New("no PriceProvider configured (required for backtest)")
    }
    // Cash schedule from actual portfolio
    cs := s.computeCashStats(allTx)

    // Simulate investing contributions (explicit deposits + inferred) into the alt symbol
    // and selling to meet explicit withdrawals.
    var evs []backtestEvent
    for _, e := range cs.depositEvents {
        evs = append(evs, backtestEvent{when: e.when, kind: "deposit", amount: e.amount})
    }
    for _, e := range cs.inferredEvents {
        evs = append(evs, backtestEvent{when: e.when, kind: "deposit", amount: e.amount})
    }
    for _, e := range cs.withdrawalEvents {
        evs = append(evs, backtestEvent{when: e.when, kind: "withdrawal", amount: e.amount})
    }
    insertionSortEvents(evs)

    // helpers for pricing on date
    getOn := func(d time.Time) (float64, time.Time, error) {
        if hp, ok := s.prices.(HistoryProvider); ok {
            // Toggle basis when supported by provider
            if yp, ok2 := s.prices.(*YahooProvider); ok2 && (priceBasis == "open" || priceBasis == "close") {
                p, asOf, err := yp.GetPriceOnBasis(symbol, d, priceBasis)
                if err == nil && p > 0 {
                    return p, asOf, nil
                }
            } else {
                p, asOf, err := hp.GetPriceOn(symbol, d)
                if err == nil && p > 0 {
                    return p, asOf, nil
                }
            }
        }
        p, asOf, err := s.prices.GetPrice(symbol)
        return p, asOf, err
    }

    var shares float64
    mult := multiplierForSymbol(symbol)
    rateSymToRef := s.rate(symbolCCY)
    if rateSymToRef <= 0 {
        rateSymToRef = 1.0
    }
    var dbg BacktestDebug
    // Track alternate equity (ref ccy) over daily history to compute max drop
    altPeak := 0.0
    altMaxDrop := 0.0 // negative percentage, e.g., -20.5
    if hp, ok := s.prices.(HistoryProvider); ok && len(evs) > 0 {
        // Group events by UTC day
        evByDay := map[time.Time][]backtestEvent{}
        start := time.Date(evs[0].when.Year(), evs[0].when.Month(), evs[0].when.Day(), 0, 0, 0, 0, time.UTC)
        for _, e := range evs {
            d := time.Date(e.when.Year(), e.when.Month(), e.when.Day(), 0, 0, 0, 0, time.UTC)
            evByDay[d] = append(evByDay[d], e)
            if d.Before(start) { start = d }
        }
        today := time.Now().UTC()
        for d := start; !d.After(today); d = d.AddDate(0, 0, 1) {
            // Daily price on chosen basis
            price, asOf, err := func() (float64, time.Time, error) {
                if yp, ok2 := s.prices.(*YahooProvider); ok2 && (priceBasis == "open" || priceBasis == "close") {
                    return yp.GetPriceOnBasis(symbol, d, priceBasis)
                }
                return hp.GetPriceOn(symbol, d)
            }()
            if err != nil || price <= 0 {
                continue
            }
            // Process any events on this day at this day's price
            if dayEvs, ok := evByDay[d]; ok {
                for _, e := range dayEvs {
                    var sharesDelta float64
                    switch e.kind {
                    case "deposit":
                        amtSym := e.amount / rateSymToRef
                        denom := price * mult
                        if denom <= 0 { denom = price }
                        sharesDelta = amtSym / denom
                        shares += sharesDelta
                    case "withdrawal":
                        amtSym := e.amount / rateSymToRef
                        denom := price * mult
                        if denom <= 0 { denom = price }
                        qty := amtSym / denom
                        sharesDelta = -qty
                        shares -= qty
                        if shares < 0 { shares = 0 }
                    }
                    if debug {
                        equityRef := shares * price * mult * rateSymToRef
                        dbg.Events = append(dbg.Events, BacktestEventDebug{
                            When:        e.when,
                            Kind:        e.kind,
                            AmountRef:   e.amount,
                            Price:       price,
                            PriceAsOf:   asOf,
                            SharesDelta: sharesDelta,
                            SharesTotal: shares,
                            EquityRef:   equityRef,
                        })
                    }
                }
            }
            // End-of-day equity and drawdown update
            equityRef := shares * price * mult * rateSymToRef
            if equityRef > altPeak { altPeak = equityRef }
            if altPeak > 0 {
                dd := (equityRef/altPeak - 1.0) * 100.0
                if dd < altMaxDrop { altMaxDrop = dd }
            }
        }
    } else {
        // Fallback: process only on event dates and final as-of
        for _, e := range evs {
            price, asOf, err := getOn(e.when)
            if err != nil || price <= 0 { continue }
            var sharesDelta float64
            switch e.kind {
            case "deposit":
                amtSym := e.amount / rateSymToRef
                denom := price * mult
                if denom <= 0 { denom = price }
                sharesDelta = amtSym / denom
                shares += sharesDelta
            case "withdrawal":
                amtSym := e.amount / rateSymToRef
                denom := price * mult
                if denom <= 0 { denom = price }
                qty := amtSym / denom
                sharesDelta = -qty
                shares -= qty
                if shares < 0 { shares = 0 }
            }
            equityRef := shares * price * mult * rateSymToRef
            if equityRef > altPeak { altPeak = equityRef }
            if altPeak > 0 {
                dd := (equityRef/altPeak - 1.0) * 100.0
                if dd < altMaxDrop { altMaxDrop = dd }
            }
            if debug {
                dbg.Events = append(dbg.Events, BacktestEventDebug{
                    When:        e.when,
                    Kind:        e.kind,
                    AmountRef:   e.amount,
                    Price:       price,
                    PriceAsOf:   asOf,
                    SharesDelta: sharesDelta,
                    SharesTotal: shares,
                    EquityRef:   equityRef,
                })
            }
        }
    }
    curPrice, _, err := s.prices.GetPrice(symbol)
    if err != nil || curPrice <= 0 {
        return BacktestResponse{}, errors.New("failed to price backtest symbol")
    }
    // Alt equity in ref currency
    altEquity := shares * curPrice * mult * rateSymToRef
    // Include final point in drawdown
    if altEquity > altPeak { altPeak = altEquity }
    if altPeak > 0 {
        dd := (altEquity/altPeak - 1.0) * 100.0
        if dd < altMaxDrop { altMaxDrop = dd }
    }

    // Compare vs contributions
    altPL := altEquity - cs.effectiveIn
    altPct := 0.0
    if cs.peakContrib > 0 {
        altPct = (altPL / cs.peakContrib) * 100.0
    }

    // Current portfolio P/L using our summary computation
    sum, err := s.computeSummaryFromTxs(allTx)
    if err != nil {
        return BacktestResponse{}, err
    }

    // Compute current portfolio max drop (drawdown) over time sampled by dates of transactions
    currentMaxDrop := 0.0 // negative percentage
    if s.prices != nil {
        // Sort transactions chronologically with inflows before outflows on same date
        xs := make([]Transaction, len(allTx))
        copy(xs, allTx)
        insertionSort(xs, func(a, b Transaction) bool {
            if a.Date.Before(b.Date) { return true }
            if a.Date.After(b.Date) { return false }
            // inflows before outflows at equal timestamps (reuse logic)
            deltaA := func(tx Transaction) float64 {
                switch tx.TradeType {
                case TradeTypeBuy:
                    amt := tx.Total; if amt < 0 { amt = -amt }
                    return -amt * s.rate(tx.Currency)
                case TradeTypeSell, TradeTypeDividend:
                    amt := tx.Total; if amt < 0 { amt = -amt }
                    return +amt * s.rate(tx.Currency)
                case TradeTypeCash:
                    return tx.Total * s.rate(tx.Currency)
                default:
                    return 0
                }
            }
            da := deltaA(a); db := deltaA(b)
            if da == db { return a.ID < b.ID }
            return da > db
        })

        type agg struct{
            shares float64
            ccy    string
        }
        holdings := map[string]*agg{}
        cash := 0.0 // in ref ccy

        // cache for historical prices by day
        type key struct{ sym string; y int; m int; d int; basis string }
        priceCache := map[key]float64{}
        asOfCache := map[key]time.Time{}
        getOn2 := func(sym string, d time.Time) (float64, time.Time, error) {
            k := key{sym: sym, y: d.Year(), m: int(d.Month()), d: d.Day(), basis: priceBasis}
            if p, ok := priceCache[k]; ok {
                return p, asOfCache[k], nil
            }
            var p float64
            var as time.Time
            var err error
            if hp, ok := s.prices.(HistoryProvider); ok {
                if yp, ok2 := s.prices.(*YahooProvider); ok2 && (priceBasis == "open" || priceBasis == "close") {
                    p, as, err = yp.GetPriceOnBasis(sym, d, priceBasis)
                } else {
                    p, as, err = hp.GetPriceOn(sym, d)
                }
            } else {
                p, as, err = s.prices.GetPrice(sym)
            }
            if err == nil && p > 0 {
                priceCache[k] = p
                asOfCache[k] = as
            }
            return p, as, err
        }

        // helper to compute equity at a date
        computeEquityAt := func(day time.Time) float64 {
            total := cash
            for sym, a := range holdings {
                if a.shares <= 0 { continue }
                p, _, err := getOn2(sym, day)
                if err != nil || p <= 0 { continue }
                mult := multiplierForSymbol(sym)
                total += a.shares * p * mult * s.rate(a.ccy)
            }
            return total
        }

        // Iterate, injecting inferred deposits to keep cash non-negative as in cashStats
        var curDay time.Time
        haveDay := false
        peak := 0.0
        updateDraw := func(day time.Time) {
            eq := computeEquityAt(day)
            if eq > peak { peak = eq }
            if peak > 0 {
                dd := (eq/peak - 1.0) * 100.0
                if dd < currentMaxDrop { currentMaxDrop = dd }
            }
        }
        for i, tx := range xs {
            // day change: finalize previous day equity
            if !haveDay || !sameYMD(curDay, tx.Date) {
                if haveDay {
                    updateDraw(curDay)
                }
                curDay = tx.Date
                haveDay = true
            }
            // Compute cash delta for this tx
            delta := 0.0
            switch tx.TradeType {
            case TradeTypeBuy:
                amt := tx.Total; if amt < 0 { amt = -amt }
                delta = -amt * s.rate(tx.Currency)
            case TradeTypeSell:
                amt := tx.Total; if amt < 0 { amt = -amt }
                delta = +amt * s.rate(tx.Currency)
            case TradeTypeDividend:
                amt := tx.Total; if amt < 0 { amt = -amt }
                delta = +amt * s.rate(tx.Currency)
            case TradeTypeCash:
                delta = tx.Total * s.rate(tx.Currency)
            }
            // Inject inferred cash if needed before applying delta
            if cash+delta < 0 {
                need := -(cash + delta)
                cash += need
            }
            // Apply holdings change
            switch tx.TradeType {
            case TradeTypeBuy:
                a := holdings[tx.Symbol]
                if a == nil { a = &agg{}; holdings[tx.Symbol] = a }
                if tx.Currency != "" { a.ccy = strings.ToUpper(tx.Currency) }
                a.shares += tx.Shares
            case TradeTypeSell:
                a := holdings[tx.Symbol]
                if a == nil { a = &agg{}; holdings[tx.Symbol] = a }
                if tx.Currency != "" { a.ccy = strings.ToUpper(tx.Currency) }
                a.shares -= tx.Shares
                if a.shares < 0 { a.shares = 0 }
            case TradeTypeDividend:
                // no change to shares
            case TradeTypeCash:
                // already reflected via delta
            }
            // Apply cash change
            cash += delta

            // If last tx overall, close day
            if i == len(xs)-1 {
                updateDraw(curDay)
            }
        }

        // Also include an as-of evaluation (today) if we have any holdings
        if haveDay {
            today := time.Now().UTC()
            updateDraw(today)
        }
    }

    resp := BacktestResponse{
        Symbol:           strings.ToUpper(strings.TrimSpace(symbol)),
        AsOf:             sum.AsOf,
        RefCurrency:      s.refCCY,
        AltPL:            altPL,
        AltPLPercent:     altPct,
        AltMaxDropPercent: altMaxDrop,
        CurrentPL:        sum.TotalUnrealizedPL,
        CurrentPLPercent: sum.TotalUnrealizedPLPerc,
        CurrentMaxDropPercent: currentMaxDrop,
    }
    if debug {
        resp.Debug = &dbg
    }
    return resp, nil
}

func insertionSortEvents(xs []backtestEvent) {
    less := func(a, b backtestEvent) bool {
        if a.when.Before(b.when) {
            return true
        }
        if a.when.After(b.when) {
            return false
        }
        // inflows before outflows on same day
        if a.kind == b.kind {
            return false
        }
        if a.kind == "deposit" && b.kind == "withdrawal" {
            return true
        }
        return false
    }
    for i := 1; i < len(xs); i++ {
        j := i
        for j > 0 && less(xs[j], xs[j-1]) {
            xs[j], xs[j-1] = xs[j-1], xs[j]
            j--
        }
    }
}

type backtestEvent struct {
    when   time.Time
    kind   string // deposit | withdrawal
    amount float64
}
