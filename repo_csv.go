package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

/*
CSV layout

portfolios.csv
id,name,base_ccy,created_at,updated_at

transactions.csv
id,portfolio_id,symbol,trade_type,currency,shares,price,fee,date,total,created_at,updated_at

Notes:
- date = "2006-01-02" (day precision)
- created_at/updated_at = RFC3339Nano
- We keep an in-memory index and write the entire file atomically after each mutation.
*/

const (
	txDateLayout = "2006-01-02"
	tsLayout     = time.RFC3339Nano
)

type csvStore struct {
	dir    string
	pfPath string
	txPath string

	mu           sync.RWMutex
	portfolios   map[string]Portfolio
	transactions map[string]Transaction // by txID
}

func NewCSVStore(dir string) (*csvStore, error) {
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &csvStore{
		dir:          dir,
		pfPath:       filepath.Join(dir, "portfolios.csv"),
		txPath:       filepath.Join(dir, "transactions.csv"),
		portfolios:   map[string]Portfolio{},
		transactions: map[string]Transaction{},
	}
	if err := s.ensureFiles(); err != nil {
		return nil, err
	}
	if err := s.loadPortfolios(); err != nil {
		return nil, err
	}
	if err := s.loadTransactions(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *csvStore) ensureFiles() error {
	// portfolios.csv
	if _, err := os.Stat(s.pfPath); errors.Is(err, os.ErrNotExist) {
		if err := atomicWriteCSV(s.pfPath, [][]string{
			{"id", "name", "base_ccy", "created_at", "updated_at"},
		}); err != nil {
			return err
		}
	}
	// transactions.csv
	if _, err := os.Stat(s.txPath); errors.Is(err, os.ErrNotExist) {
		if err := atomicWriteCSV(s.txPath, [][]string{
			{"id", "portfolio_id", "symbol", "trade_type", "currency", "shares", "price", "fee", "date", "total", "created_at", "updated_at"},
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *csvStore) loadPortfolios() error {
	f, err := os.Open(s.pfPath)
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	if err != nil {
		return err
	}
	if len(rows) <= 1 {
		return nil
	}
	for i := 1; i < len(rows); i++ {
		row := rows[i]
		if len(row) < 5 {
			continue
		}
		createdAt, _ := time.Parse(tsLayout, row[3])
		updatedAt, _ := time.Parse(tsLayout, row[4])
		p := Portfolio{
			ID:        row[0],
			Name:      row[1],
			BaseCCY:   row[2],
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		}
		s.portfolios[p.ID] = p
	}
	return nil
}

func (s *csvStore) loadTransactions() error {
	f, err := os.Open(s.txPath)
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	if err != nil {
		return err
	}
	if len(rows) <= 1 {
		return nil
	}
	for i := 1; i < len(rows); i++ {
		row := rows[i]
		if len(row) < 12 {
			continue
		}
		shares, _ := strconv.ParseFloat(row[5], 64)
		price, _ := strconv.ParseFloat(row[6], 64)
		fee, _ := strconv.ParseFloat(row[7], 64)
		total, _ := strconv.ParseFloat(row[9], 64)

		// date: prefer 2006-01-02; fallback to RFC3339; then "2006/01/02" if needed
		var dt time.Time
		var e error
		for _, layout := range []string{txDateLayout, time.RFC3339, payloadDateLayout} {
			dt, e = time.Parse(layout, row[8])
			if e == nil {
				break
			}
		}

		createdAt, _ := time.Parse(tsLayout, row[10])
		updatedAt, _ := time.Parse(tsLayout, row[11])

		tx := Transaction{
			ID:          row[0],
			PortfolioID: row[1],
			Symbol:      row[2],
			TradeType:   TradeType(row[3]),
			Currency:    row[4],
			Shares:      shares,
			Price:       price,
			Fee:         fee,
			Date:        dt,
			Total:       total,
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
		}
		s.transactions[tx.ID] = tx
	}
	return nil
}

func (s *csvStore) savePortfoliosLocked() error {
	rows := make([][]string, 0, len(s.portfolios)+1)
	rows = append(rows, []string{"id", "name", "base_ccy", "created_at", "updated_at"})
	for _, p := range s.portfolios {
		rows = append(rows, []string{
			p.ID, p.Name, p.BaseCCY,
			p.CreatedAt.Format(tsLayout),
			p.UpdatedAt.Format(tsLayout),
		})
	}
	return atomicWriteCSV(s.pfPath, rows)
}

func (s *csvStore) saveTransactionsLocked() error {
	rows := make([][]string, 0, len(s.transactions)+1)
	rows = append(rows, []string{"id", "portfolio_id", "symbol", "trade_type", "currency", "shares", "price", "fee", "date", "total", "created_at", "updated_at"})
	for _, tx := range s.transactions {
		rows = append(rows, []string{
			tx.ID,
			tx.PortfolioID,
			tx.Symbol,
			string(tx.TradeType),
			tx.Currency,
			fmt.Sprintf("%.10f", tx.Shares),
			fmt.Sprintf("%.10f", tx.Price),
			fmt.Sprintf("%.10f", tx.Fee),
			tx.Date.Format(txDateLayout),
			fmt.Sprintf("%.10f", tx.Total),
			tx.CreatedAt.Format(tsLayout),
			tx.UpdatedAt.Format(tsLayout),
		})
	}
	return atomicWriteCSV(s.txPath, rows)
}

func atomicWriteCSV(path string, rows [][]string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "tmp-*.csv")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	w := csv.NewWriter(tmp)
	if err := w.WriteAll(rows); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	w.Flush()
	if err := w.Error(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}

/* ======================== Portfolio repo ======================== */

type csvPortfolioRepo struct{ s *csvStore }

func NewCSVPortfolioRepo(s *csvStore) *csvPortfolioRepo { return &csvPortfolioRepo{s: s} }

func (r *csvPortfolioRepo) Create(p Portfolio) (Portfolio, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	r.s.portfolios[p.ID] = p
	return p, r.s.savePortfoliosLocked()
}

func (r *csvPortfolioRepo) GetByID(id string) (Portfolio, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()
	p, ok := r.s.portfolios[id]
	if !ok {
		return Portfolio{}, ErrNotFound
	}
	return p, nil
}

func (r *csvPortfolioRepo) List() ([]Portfolio, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()
	out := make([]Portfolio, 0, len(r.s.portfolios))
	for _, p := range r.s.portfolios {
		out = append(out, p)
	}
	return out, nil
}

func (r *csvPortfolioRepo) Update(p Portfolio) (Portfolio, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if _, ok := r.s.portfolios[p.ID]; !ok {
		return Portfolio{}, ErrNotFound
	}
	// ensure UpdatedAt is respected by caller (service sets it); still bump to now for safety
	p.UpdatedAt = time.Now()
	r.s.portfolios[p.ID] = p
	return p, r.s.savePortfoliosLocked()
}

func (r *csvPortfolioRepo) Delete(id string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if _, ok := r.s.portfolios[id]; !ok {
		return ErrNotFound
	}
	delete(r.s.portfolios, id)
	// cascade delete transactions
	for txID, tx := range r.s.transactions {
		if tx.PortfolioID == id {
			delete(r.s.transactions, txID)
		}
	}
	if err := r.s.savePortfoliosLocked(); err != nil {
		return err
	}
	return r.s.saveTransactionsLocked()
}

/* ======================== Transaction repo ======================== */

type csvTransactionRepo struct{ s *csvStore }

func NewCSVTransactionRepo(s *csvStore) *csvTransactionRepo { return &csvTransactionRepo{s: s} }

func (r *csvTransactionRepo) Create(portfolioID string, tx Transaction) (Transaction, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if _, ok := r.s.portfolios[portfolioID]; !ok {
		return Transaction{}, ErrPortfolioNotFound
	}
	r.s.transactions[tx.ID] = tx
	return tx, r.s.saveTransactionsLocked()
}

func (r *csvTransactionRepo) CreateBatch(portfolioID string, txs []Transaction) ([]Transaction, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if _, ok := r.s.portfolios[portfolioID]; !ok {
		return nil, ErrPortfolioNotFound
	}
	for _, tx := range txs {
		r.s.transactions[tx.ID] = tx
	}
	return txs, r.s.saveTransactionsLocked()
}

func (r *csvTransactionRepo) GetByID(portfolioID, txID string) (Transaction, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()
	if _, ok := r.s.portfolios[portfolioID]; !ok {
		return Transaction{}, ErrPortfolioNotFound
	}
	tx, ok := r.s.transactions[txID]
	if !ok || tx.PortfolioID != portfolioID {
		return Transaction{}, ErrNotFound
	}
	return tx, nil
}

func (r *csvTransactionRepo) List(portfolioID string, filter ListFilter) ([]Transaction, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()
	if _, ok := r.s.portfolios[portfolioID]; !ok {
		return nil, ErrPortfolioNotFound
	}
	out := make([]Transaction, 0, 32)
	for _, tx := range r.s.transactions {
		if tx.PortfolioID != portfolioID {
			continue
		}
		if filter.Symbol != "" && !equalFold(filter.Symbol, tx.Symbol) {
			continue
		}
		out = append(out, tx)
	}
	switch filter.Sort {
	case "date_asc":
		insertionSort(out, func(a, b Transaction) bool { return a.Date.Before(b.Date) })
	case "date_desc":
		insertionSort(out, func(a, b Transaction) bool { return a.Date.After(b.Date) })
	}
	start := filter.Offset
	if start > len(out) {
		return []Transaction{}, nil
	}
	end := len(out)
	if filter.Limit > 0 && start+filter.Limit < end {
		end = start + filter.Limit
	}
	return out[start:end], nil
}

func (r *csvTransactionRepo) Update(portfolioID string, tx Transaction) (Transaction, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if _, ok := r.s.portfolios[portfolioID]; !ok {
		return Transaction{}, ErrPortfolioNotFound
	}
	old, ok := r.s.transactions[tx.ID]
	if !ok || old.PortfolioID != portfolioID {
		return Transaction{}, ErrNotFound
	}
	tx.UpdatedAt = time.Now()
	r.s.transactions[tx.ID] = tx
	return tx, r.s.saveTransactionsLocked()
}

func (r *csvTransactionRepo) Delete(portfolioID, txID string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if _, ok := r.s.portfolios[portfolioID]; !ok {
		return ErrPortfolioNotFound
	}
	tx, ok := r.s.transactions[txID]
	if !ok || tx.PortfolioID != portfolioID {
		return ErrNotFound
	}
	delete(r.s.transactions, txID)
	return r.s.saveTransactionsLocked()
}
