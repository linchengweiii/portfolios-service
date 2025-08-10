package main

import (
	"sync"
	"time"
)

// ===== In-memory adapters =====

type memoryStore struct {
	mu           sync.RWMutex
	portfolios   map[string]Portfolio
	transactions map[string]map[string]Transaction // portfolioID -> txID -> tx
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		portfolios:   make(map[string]Portfolio),
		transactions: make(map[string]map[string]Transaction),
	}
}

/* ---- Portfolio repo ---- */

type memoryPortfolioRepo struct{ s *memoryStore }

func NewMemoryPortfolioRepo(s *memoryStore) *memoryPortfolioRepo { return &memoryPortfolioRepo{s: s} }

func (r *memoryPortfolioRepo) Create(p Portfolio) (Portfolio, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	r.s.portfolios[p.ID] = p
	if _, ok := r.s.transactions[p.ID]; !ok {
		r.s.transactions[p.ID] = make(map[string]Transaction)
	}
	return p, nil
}

func (r *memoryPortfolioRepo) GetByID(id string) (Portfolio, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()
	p, ok := r.s.portfolios[id]
	if !ok {
		return Portfolio{}, ErrNotFound
	}
	return p, nil
}

func (r *memoryPortfolioRepo) List() ([]Portfolio, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()
	out := make([]Portfolio, 0, len(r.s.portfolios))
	for _, p := range r.s.portfolios {
		out = append(out, p)
	}
	return out, nil
}

func (r *memoryPortfolioRepo) Update(p Portfolio) (Portfolio, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if _, ok := r.s.portfolios[p.ID]; !ok {
		return Portfolio{}, ErrNotFound
	}
	p.UpdatedAt = time.Now()
	r.s.portfolios[p.ID] = p
	return p, nil
}

func (r *memoryPortfolioRepo) Delete(id string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if _, ok := r.s.portfolios[id]; !ok {
		return ErrNotFound
	}
	delete(r.s.portfolios, id)
	delete(r.s.transactions, id)
	return nil
}

/* ---- Transaction repo ---- */

type memoryTransactionRepo struct{ s *memoryStore }

func NewMemoryTransactionRepo(s *memoryStore) *memoryTransactionRepo { return &memoryTransactionRepo{s: s} }

func (r *memoryTransactionRepo) ensurePortfolio(portfolioID string) error {
	if _, ok := r.s.portfolios[portfolioID]; !ok {
		return ErrPortfolioNotFound
	}
	if _, ok := r.s.transactions[portfolioID]; !ok {
		r.s.transactions[portfolioID] = make(map[string]Transaction)
	}
	return nil
}

func (r *memoryTransactionRepo) Create(portfolioID string, tx Transaction) (Transaction, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if err := r.ensurePortfolio(portfolioID); err != nil {
		return Transaction{}, err
	}
	r.s.transactions[portfolioID][tx.ID] = tx
	return tx, nil
}

func (r *memoryTransactionRepo) CreateBatch(portfolioID string, txs []Transaction) ([]Transaction, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if err := r.ensurePortfolio(portfolioID); err != nil {
		return nil, err
	}
	for _, tx := range txs {
		r.s.transactions[portfolioID][tx.ID] = tx
	}
	return txs, nil
}

func (r *memoryTransactionRepo) GetByID(portfolioID, txID string) (Transaction, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()
	pool, ok := r.s.transactions[portfolioID]
	if !ok {
		return Transaction{}, ErrPortfolioNotFound
	}
	tx, ok := pool[txID]
	if !ok {
		return Transaction{}, ErrNotFound
	}
	return tx, nil
}

func (r *memoryTransactionRepo) List(portfolioID string, filter ListFilter) ([]Transaction, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()
	pool, ok := r.s.transactions[portfolioID]
	if !ok {
		return nil, ErrPortfolioNotFound
	}
	out := make([]Transaction, 0, len(pool))
	for _, tx := range pool {
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

func (r *memoryTransactionRepo) Update(portfolioID string, tx Transaction) (Transaction, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	pool, ok := r.s.transactions[portfolioID]
	if !ok {
		return Transaction{}, ErrPortfolioNotFound
	}
	if _, ok := pool[tx.ID]; !ok {
		return Transaction{}, ErrNotFound
	}
	tx.UpdatedAt = time.Now()
	pool[tx.ID] = tx
	return tx, nil
}

func (r *memoryTransactionRepo) Delete(portfolioID, txID string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	pool, ok := r.s.transactions[portfolioID]
	if !ok {
		return ErrPortfolioNotFound
	}
	if _, ok := pool[txID]; !ok {
		return ErrNotFound
	}
	delete(pool, txID)
	return nil
}

