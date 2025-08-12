package main

import (
	"log"
	"net/http"
	"os"
	"strings"
)

func main() {
	var pfRepo PortfolioRepository
	var txRepo TransactionRepository

	repoKind := strings.ToLower(strings.TrimSpace(os.Getenv("REPO_KIND")))
	switch repoKind {
	case "memory":
		// Explicitly choose in-memory
		mem := newMemoryStore()
		pfRepo = NewMemoryPortfolioRepo(mem)
		txRepo = NewMemoryTransactionRepo(mem)
	default:
		// Default: CSV repo
		dataDir := os.Getenv("DATA_DIR")
		if dataDir == "" {
			dataDir = "./data"
		}
		store, err := NewCSVStore(dataDir)
		if err != nil {
			log.Fatalf("init csv store: %v", err)
		}
		pfRepo = NewCSVPortfolioRepo(store)
		txRepo = NewCSVTransactionRepo(store)
	}

	// Price provider selection
	var priceProv PriceProvider

	prov := strings.ToLower(strings.TrimSpace(os.Getenv("PRICE_PROVIDER")))
	switch prov {
	case "alphavantage", "alpha", "av":
		ap, err := NewAlphaVantageProviderFromEnv()
		if err != nil {
			log.Printf("Alpha Vantage not configured (%v); falling back to Yahoo.", err)
			priceProv = NewYahooProvider()
		} else {
			priceProv = ap
		}
	default: // default to Yahoo
		priceProv = NewYahooProvider()
	}

	pfSvc := NewPortfolioService(pfRepo)
	txSvc := NewTransactionService(txRepo, pfRepo, priceProv)

	srv := NewServer(pfSvc, txSvc)

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", srv))
}
