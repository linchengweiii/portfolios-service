package main

import "errors"

// ===== Ports (interfaces) =====

type PortfolioRepository interface {
	Create(p Portfolio) (Portfolio, error)
	GetByID(id string) (Portfolio, error)
	List() ([]Portfolio, error)
	Update(p Portfolio) (Portfolio, error)
	Delete(id string) error
}

type ListFilter struct {
	Symbol string
	Limit  int
	Offset int
	Sort   string // "date_asc" | "date_desc" | ""
}

type TransactionRepository interface {
	Create(portfolioID string, tx Transaction) (Transaction, error)
	CreateBatch(portfolioID string, txs []Transaction) ([]Transaction, error)
	GetByID(portfolioID, txID string) (Transaction, error)
	List(portfolioID string, filter ListFilter) ([]Transaction, error)
	Update(portfolioID string, tx Transaction) (Transaction, error)
	Delete(portfolioID, txID string) error
}

// Common errors
var ErrNotFound = errors.New("not found")
var ErrPortfolioNotFound = errors.New("portfolio not found")

/* ======================== small helpers ======================== */
func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range len(a) {
		ai, bi := a[i], b[i]
		if ai >= 'A' && ai <= 'Z' {
			ai += 32
		}
		if bi >= 'A' && bi <= 'Z' {
			bi += 32
		}
		if ai != bi {
			return false
		}
	}
	return true
}

func insertionSort(xs []Transaction, less func(a, b Transaction) bool) {
	for i := 1; i < len(xs); i++ {
		j := i
		for j > 0 && less(xs[j], xs[j-1]) {
			xs[j], xs[j-1] = xs[j-1], xs[j]
			j--
		}
	}
}
