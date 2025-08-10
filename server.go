package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// ===== HTTP adapter =====

type Server struct {
	pf  *PortfolioService
	tx  *TransactionService
	mux *http.ServeMux
}

func NewServer(pf *PortfolioService, tx *TransactionService) *Server {
	s := &Server{pf: pf, tx: tx, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) routes() {
	// Global endpoints (all portfolios)
	s.mux.HandleFunc("/allocations", s.handleAllocationsAll) // GET
	s.mux.HandleFunc("/summary", s.handleSummaryAll)         // GET

	// Root collection for portfolios (exact path)
	s.mux.HandleFunc("/portfolios", s.handlePortfolios)

	// Single subtree handler for everything under /portfolios/
	s.mux.HandleFunc("/portfolios/", s.handlePortfoliosSub)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Simple JSON-only API
	w.Header().Set("Content-Type", "application/json")
	s.mux.ServeHTTP(w, r)
}

/* ======= Global endpoints ======= */

// GET /allocations?basis=invested|market_value  (across ALL portfolios)
func (s *Server) handleAllocationsAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	basis := r.URL.Query().Get("basis")
	if basis == "" {
		basis = "invested"
	}
	out, err := s.tx.ComputeAllocationsAll(basis)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// GET /summary  (across ALL portfolios)
func (s *Server) handleSummaryAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	out, err := s.tx.ComputeSummaryAll()
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

/* ======= Portfolios root ======= */

func (s *Server) handlePortfolios(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		defer r.Body.Close()
		var dto portfolioDTO
		if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
			httpError(w, http.StatusBadRequest, "invalid payload: "+err.Error())
			return
		}
		out, err := s.pf.Create(dto)
		if err != nil {
			httpError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, out)
	case http.MethodGet:
		out, err := s.pf.List()
		if err != nil {
			httpError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, out)
	default:
		httpError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

/* ======= Portfolios subtree ======= */

func (s *Server) handlePortfoliosSub(w http.ResponseWriter, r *http.Request) {
	// Path starts with /portfolios/
	rest := strings.TrimPrefix(r.URL.Path, "/portfolios/")
	rest = strings.TrimSuffix(rest, "/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}

	parts := strings.Split(rest, "/")

	// Case A: /portfolios/{id}
	if len(parts) == 1 {
		id := parts[0]
		switch r.Method {
		case http.MethodGet:
			p, err := s.pf.Get(id)
			if err != nil {
				status := http.StatusInternalServerError
				if err == ErrNotFound {
					status = http.StatusNotFound
				}
				httpError(w, status, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, p)
		case http.MethodPut:
			defer r.Body.Close()
			var dto portfolioDTO
			if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
				httpError(w, http.StatusBadRequest, "invalid payload: "+err.Error())
				return
			}
			p, err := s.pf.Update(id, dto)
			if err != nil {
				status := http.StatusBadRequest
				if err == ErrNotFound {
					status = http.StatusNotFound
				}
				httpError(w, status, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, p)
		case http.MethodDelete:
			if err := s.pf.Delete(id); err != nil {
				status := http.StatusInternalServerError
				if err == ErrNotFound {
					status = http.StatusNotFound
				}
				httpError(w, status, err.Error())
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			httpError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	// Case B: /portfolios/{id}/transactions[/...]
	if len(parts) >= 2 && parts[1] == "transactions" {
		pfID := parts[0]

		// Collection: /portfolios/{id}/transactions
		if len(parts) == 2 {
			switch r.Method {
			case http.MethodPost:
				s.createTx(pfID, w, r)
			case http.MethodGet:
				s.listTx(pfID, w, r)
			default:
				httpError(w, http.StatusMethodNotAllowed, "method not allowed")
			}
			return
		}

		// Item: /portfolios/{id}/transactions/{txID}
		if len(parts) == 3 {
			txID := parts[2]
			switch r.Method {
			case http.MethodGet:
				tx, err := s.tx.Get(pfID, txID)
				if err != nil {
					status := http.StatusInternalServerError
					if err == ErrNotFound || err == ErrPortfolioNotFound {
						status = http.StatusNotFound
					}
					httpError(w, status, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, tx)
			case http.MethodPut:
				defer r.Body.Close()
				var dto transactionDTO
				if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
					httpError(w, http.StatusBadRequest, "invalid payload: "+err.Error())
					return
				}
				tx, err := s.tx.Update(pfID, txID, dto)
				if err != nil {
					status := http.StatusBadRequest
					if err == ErrNotFound || err == ErrPortfolioNotFound {
						status = http.StatusNotFound
					}
					httpError(w, status, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, tx)
			case http.MethodDelete:
				if err := s.tx.Delete(pfID, txID); err != nil {
					status := http.StatusInternalServerError
					if err == ErrNotFound || err == ErrPortfolioNotFound {
						status = http.StatusNotFound
					}
					httpError(w, status, err.Error())
					return
				}
				w.WriteHeader(http.StatusNoContent)
			default:
				httpError(w, http.StatusMethodNotAllowed, "method not allowed")
			}
			return
		}
	}

	// Case C: /portfolios/{id}/allocations
	if len(parts) == 2 && parts[1] == "allocations" {
		if r.Method != http.MethodGet {
			httpError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		pfID := parts[0]
		basis := r.URL.Query().Get("basis")
		if basis == "" {
			basis = "invested" // default
		}
		out, err := s.tx.ComputeAllocations(pfID, basis)
		if err != nil {
			status := http.StatusBadRequest
			if err == ErrPortfolioNotFound {
				status = http.StatusNotFound
			}
			httpError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, out)
		return
	}

	http.NotFound(w, r)
}

/* ======= Transactions helpers ======= */

func (s *Server) createTx(pfID string, w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20) // 5MB limit
	body, err := io.ReadAll(r.Body)
	if err != nil {
		httpError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}

	switch firstNonWS(body) {
	case '[':
		var payload []transactionDTO
		if err := json.Unmarshal(body, &payload); err != nil {
			httpError(w, http.StatusBadRequest, "invalid batch payload: "+err.Error())
			return
		}
		out, err := s.tx.CreateBatch(pfID, payload)
		if err != nil {
			status := http.StatusBadRequest
			if err == ErrPortfolioNotFound {
				status = http.StatusNotFound
			}
			httpError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, out)
	case '{':
		var payload transactionDTO
		if err := json.Unmarshal(body, &payload); err != nil {
			httpError(w, http.StatusBadRequest, "invalid payload: "+err.Error())
			return
		}
		out, err := s.tx.CreateOne(pfID, payload)
		if err != nil {
			status := http.StatusBadRequest
			if err == ErrPortfolioNotFound {
				status = http.StatusNotFound
			}
			httpError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, out)
	default:
		httpError(w, http.StatusBadRequest, "payload must be object or array")
	}
}

func (s *Server) listTx(pfID string, w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := atoiDefault(q.Get("limit"), 50)
	offset := atoiDefault(q.Get("offset"), 0)
	sort := q.Get("sort")
	if sort != "" && sort != "date_asc" && sort != "date_desc" {
		httpError(w, http.StatusBadRequest, "invalid sort (use date_asc|date_desc)")
		return
	}
	filter := ListFilter{
		Symbol: q.Get("symbol"), // symbol-only filtering
		Limit:  limit,
		Offset: offset,
		Sort:   sort,
	}
	items, err := s.tx.List(pfID, filter)
	if err != nil {
		status := http.StatusInternalServerError
		if err == ErrPortfolioNotFound {
			status = http.StatusNotFound
		}
		httpError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

/* ======= small helpers ======= */

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":  http.StatusText(status),
		"detail": msg,
	})
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func firstNonWS(b []byte) byte {
	for _, c := range b {
		switch c {
		case ' ', '\n', '\t', '\r':
			continue
		default:
			return c
		}
	}
	return 0
}
