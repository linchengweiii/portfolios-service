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
- trade_type: buy | sell | dividend | cash.
- date format: YYYY/MM/DD.
- For purchases, total is usually negative (cash out). The service uses ABS(total) as invested capital.

## REST API
### Portfolios

- Create: `POST /portfolios`

  ```json
  { "name": "Core US Tech" }
  ```
  Notes:
  - `base_ccy` is no longer required for creation. The service computes values in a per-request reference currency via the `ref_ccy` query param (see below). If provided on creation, `base_ccy` is stored but not used for calculations.
- List: `GET /portfolios`
- Get: `GET /portfolios/{id}`
- Update: `PUT /portfolios/{id}`
- Delete: `DELETE /portfolios/{id}`

### Transactions (under a portfolio)

- **Create (single or batch)**: `POST /portfolios/{id}/transactions`

  ```json
  {
    "symbol": "AMZN",
    "trade_type": "buy",
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

- **Per portfolio**: `GET /portfolios/{id}/allocations?basis=invested|market_value&ref_ccy=TWD|USD`
- **All portfolios**: `GET /allocations?basis=invested|market_value&ref_ccy=TWD|USD`

`ref_ccy` controls the reference currency for output and conversions. Allowed values: `TWD` or `USD` (default `TWD`).

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

### Summary

- **Global summary**: `GET /summary?ref_ccy=TWD|USD`
- **Per-portfolio summary**: `GET /portfolios/{id}/summary?ref_ccy=TWD|USD`

### Backtest

- **Global backtest**: `GET /backtest?symbol={SYMBOL}&ref_ccy=TWD|USD`
- **Per-portfolio backtest**: `GET /portfolios/{id}/backtest?symbol={SYMBOL}&ref_ccy=TWD|USD`

Optional params:
- `symbol_ccy`: currency of `{SYMBOL}` quotes (default `USD`).
- `price_basis`: `open` or `close` (default `close`; backtest only).
- `debug`: `1` to include event-by-event simulation details.
 - `ref_ccy`: output currency for calculations (`TWD` or `USD`; defaults to `TWD`).

Response shape:

```json
{
  "symbol": "QQQ",
  "as_of": "2025-08-10T00:00:00Z",
  "ref_currency": "USD",
  "alt_pl": 123.45,
  "alt_pl_percent": 3.2,
  "current_pl": 98.76,
  "current_pl_percent": 2.5
}
```

Rules:
- Deposits: invest all explicit cash deposits plus inferred deposits into `{SYMBOL}` at the date of the deposit.
- Withdrawals: sell `{SYMBOL}` to fund explicit cash withdrawals at their dates.
- Inferred deposits: computed from your actual transactions as the minimal additions needed to prevent negative cash; they are assumed to be deposited right before the buys that required them, and are invested into `{SYMBOL}` in the backtest.
- Prices: uses daily historical prices when available (Yahoo). If history is unavailable, falls back to the latest price for approximation.
- Percent basis: uses peak contributed cash (deposits − withdrawals + inferred, never below zero) as denominator to avoid extreme values after withdrawals.

```json
{
  "as_of": "2025-08-10T00:00:00Z",
  "total_invested": 4310.9,
  "total_market_value": 4520.0,
  "total_unrealized_pl": 209.1,
  "total_unrealized_pl_percent": 4.85,
  "total_unrealized_pl_percent_current": 3.10,
  "daily_pl": 12.34,
  "daily_pl_percent": 0.27,
  "balance": 3900.0,
  "cash_deposits": 7000.0,
  "cash_withdrawals": 0.0,
  "inferred_deposits": 0.0,
  "effective_cash_in": 7000.0,
  "effective_cash_in_peak": 7000.0,
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

- “Invested” (in summary) = cost of the shares you still hold: buys add cost; sells reduce cost using average cost per share. Dividends do not change invested.
- Summary P/L is **unrealized**. Realized P/L support can be added later without changing the API.
- Trade type `cash` lets you record deposits/withdrawals. It may omit `symbol`.
  - Positive `total` = deposit; negative `total` = withdrawal (values are converted to the reference currency).
- Balance in summary injects the minimal extra deposits needed so the running balance never goes below zero (buys negative, sells/dividends positive, cash deposits positive, cash withdrawals negative), sorted by date.
- Cash-based P/L:
  - P/L (summary) = MarketValue + Balance − EffectiveCashIn.
  - EffectiveCashIn = CashDeposits − CashWithdrawals + InferredDeposits.
  - P/L% (summary) = P/L / EffectiveCashIn × 100 (when denominator > 0).
- Daily P/L:
  - Sum over positions of `shares × (close_today − close_prev)` converted into the reference currency.
  - Daily P/L% = Daily P/L divided by yesterday's market value of held positions (sum of `shares × close_prev` in ref currency) × 100.
  - Requires a history-capable price provider (Yahoo). If unavailable, `daily_pl` may be omitted or zero.
  - Excludes cash flows; reflects price movement only.
- Cash stats implementation:
  - Cash deposits/withdrawals come from `trade_type = cash` only (deposits positive, withdrawals negative). Buys/sells/dividends affect balance but are not counted as deposits/withdrawals.
  - Transactions are sorted by date; for the same timestamp, inflows (sell/dividend/deposit) are applied before outflows (buy/withdrawal) to minimize temporary negative balances.
  - `inferred_deposits` is the minimal extra deposit needed so the running cash balance never goes below zero (computed after ordering). This helps when some deposits are missing from data.
- Storage is in-memory; swap to a DB by implementing the repo interfaces and wiring in `main.go`.
