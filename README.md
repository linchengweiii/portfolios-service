# Stock Portfolios Service (Go)

Track multiple portfolios and transactions with a clean, swappable architecture.  
Features:
- Multiple portfolios
- CRUD transactions (single or batch)
- Allocations by **invested** or **market value**
- **Global** allocations across all portfolios
- **Global** summary (invested, market value, overall unrealized P/L)

## Quick Start

```bash
go mod init github.com/you/stock-portfolios
go get github.com/google/uuid@latest

# Alpha Vantage key is needed for market_value & summary endpoints
export ALPHAVANTAGE_API_KEY=YOUR_KEY

go run .
# server listens on :8080
```
Prices use Alpha Vantage GLOBAL_QUOTE (free keys are typically end-of-day).
Without ALPHAVANTAGE_API_KEY, /allocations?basis=market_value and /summary will error.



## Data Model Notes

- Use symbol only (e.g., AMZN, BHP.AX, 7203.T).
- trade_type: purchase | sell | dividend.
- date format: YYYY/MM/DD.
- For purchases, total is usually negative (cash out). The service uses ABS(total) as invested capital.

## REST API
### Portfolios

- Create: `POST /portfolios`

  ```json
  { "name": "Core US Tech", "base_ccy": "USD" }
  ```
- List: `GET /portfolios`
- Get: `GET /portfolios/{id}`
- Update: `PUT /portfolios/{id}`
- Delete: `DELETE /portfolios/{id}`

### Transactions (under a portfolio)

- **Create (single or batch)**: `POST /portfolios/{id}/transactions`

  ```json
  {
    "symbol": "AMZN",
    "trade_type": "purchase",
    "currency": "USD",
    "shares": 1.234,
    "price": 210.5,
    "fee": 0.1,
    "date": "2025/08/06",
    "total": -259.11
  }
  ```

- **List**: `GET /portfolios/{id}/transactions?symbol=NVDA&sort=date_desc&limit=50&offset=0`
- **Get**: `GET /portfolios/{id}/transactions/{txID}`
- **Update**: `PUT /portfolios/{id}/transactions/{txID}`
- **Delete**: `DELETE /portfolios/{id}/transactions/{txID}`

### Allocations

- **Per portfolio**: `GET /portfolios/{id}/allocations?basis=invested|market_value`
- **All portfolios**: `GET /allocations?basis=invested|market_value`

**Response (shape):**

```json
{
  "basis": "market_value",
  "total_market_value": 12345.67,
  "as_of": "2025-08-10T00:00:00Z",
  "items": [
    { "symbol":"AMZN","shares":2.0,"invested":420.2,"market_value":426.2,"weight_percent":3.45 }
  ]
}
```

### Summary (All portfolios)

- **Global summary**: `GET /summary`

```json
{
  "as_of": "2025-08-10T00:00:00Z",
  "total_invested": 4310.9,
  "total_market_value": 4520.0,
  "total_unrealized_pl": 209.1,
  "total_unrealized_pl_percent": 4.85,
  "positions": [
    {
      "symbol": "AMZN",
      "shares": 2.0,
      "invested": 420.2,
      "market_value": 426.2,
      "unrealized_pl": 6.0,
      "unrealized_pl_percent": 1.43,
      "weight_percent_by_market_value": 9.43
    }
  ]
}
```

## Notes

- “Invested” = sum of **ABS(purchase totals)**; sells don’t reduce invested.
- Summary P/L is **unrealized**. Realized P/L support can be added later without changing the API.
- Storage is in-memory; swap to a DB by implementing the repo interfaces and wiring in `main.go`.
