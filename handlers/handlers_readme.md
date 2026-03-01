# 🎮 Handlers (`handlers/`)

Handlers are the **HTTP endpoint controllers**. They receive requests from the frontend, call the database layer, and return JSON responses.

---

## 📚 Libraries Used

| Library | Import Path | Purpose |
|---------|-------------|---------|
| `gin-gonic/gin` | `github.com/gin-gonic/gin` | Web framework - handles routing, request parsing, JSON responses |
| `golang-jwt/jwt` | `github.com/golang-jwt/jwt/v5` | Creates and validates JSON Web Tokens for auth |
| `bcrypt` | `golang.org/x/crypto/bcrypt` | Password hashing (one-way encryption) |
| `database/sql` | `database/sql` | Database connection passed to handlers |
| `encoding/json` | `encoding/json` | Parse JSON from external APIs |
| `sync` | `sync` | Thread-safe caching with mutex locks |

---

## 📁 Files Overview

| File | Purpose | Endpoints |
|------|---------|-----------|
| `users.go` | Auth & user profile | `/api/register`, `/api/login`, `/api/me` |
| `groups.go` | Group management | `/api/groups/*` |
| `expenses.go` | Expense CRUD | `/api/groups/:id/expenses/*` |
| `settlements.go` | Direct payments & balances | `/api/groups/:id/settle`, `/api/groups/:id/balances` |
| `expense_payments.go` | Partial expense payments | `/api/expenses/:id/payments/*` |
| `activity.go` | Activity feed | `/api/groups/:id/activities` |
| `suggestions.go` | Feature suggestions | `/api/suggestions/*` |
| `currency.go` | Currency conversion | `/api/currency/*` |

---

## 🔧 users.go

### Handler Struct

```go
type UserHandler struct {
    DB *sql.DB  // Database connection
}
```

### Variables

| Variable | Type | Purpose |
|----------|------|---------|
| `allowedEmails` | `map[string]bool` | Whitelist of emails allowed to register |

### Endpoints

| Method | Endpoint | Function | Description |
|--------|----------|----------|-------------|
| POST | `/api/register` | `Register` | Creates new user (checks whitelist, hashes password) |
| POST | `/api/login` | `Login` | Validates credentials, returns JWT token |
| GET | `/api/me` | `GetProfile` | Returns current user's profile |
| GET | `/api/users` | `GetAllUsers` | Returns all users (for member selection) |
| PUT | `/api/me/theme` | `UpdateTheme` | Updates user's theme preference |

### Helper Functions

| Function | Parameters | Returns | Description |
|----------|------------|---------|-------------|
| `generateJWT` | `userID int` | `string, error` | Creates a signed JWT token (24h expiry) |

---

## 🔧 groups.go

### Endpoints

| Method | Endpoint | Function | Description |
|--------|----------|----------|-------------|
| POST | `/api/groups` | `CreateGroup` | Creates group, adds creator as member |
| GET | `/api/groups` | `GetGroups` | Lists user's groups with their balance |
| GET | `/api/groups/:id` | `GetGroup` | Gets single group with members |
| POST | `/api/groups/:id/members` | `AddMember` | Adds user to group |
| DELETE | `/api/groups/:id/members/:userId` | `RemoveMember` | Removes user from group |

---

## 🔧 expenses.go

### Endpoints

| Method | Endpoint | Function | Description |
|--------|----------|----------|-------------|
| POST | `/api/groups/:id/expenses` | `CreateExpense` | Creates expense with splits |
| GET | `/api/groups/:id/expenses` | `GetExpenses` | Lists all expenses in group |
| GET | `/api/groups/:id/expenses/:expenseId` | `GetExpense` | Gets single expense details |
| DELETE | `/api/groups/:id/expenses/:expenseId` | `DeleteExpense` | Removes an expense |

### Split Types

| Type | Description |
|------|-------------|
| `equal` | Amount divided equally among all group members |
| `exact` | Each user's share specified in `split_with` array |
| `percentage` | Each user's percentage specified, converted to amounts |

---

## 🔧 settlements.go

### Endpoints

| Method | Endpoint | Function | Description |
|--------|----------|----------|-------------|
| POST | `/api/groups/:id/settle` | `CreateSettlement` | Records direct payment between users |
| GET | `/api/groups/:id/settlements` | `GetSettlements` | Lists all settlements in group |
| GET | `/api/groups/:id/balances` | `GetBalances` | **Key!** Who owes whom in the group |
| GET | `/api/groups/:id/my-balance` | `GetMyBalance` | Current user's balance in group |
| GET | `/api/me/debt-overview` | `GetDebtOverview` | User's debts across ALL groups |
| GET | `/api/me/payment-history` | `GetPaymentHistory` | User's payment history |

---

## 🔧 expense_payments.go

### Endpoints

| Method | Endpoint | Function | Description |
|--------|----------|----------|-------------|
| POST | `/api/expenses/:id/payments` | `CreatePayment` | Record payment toward expense |
| GET | `/api/expenses/:id/payments` | `GetPayments` | List payments on an expense |
| DELETE | `/api/expenses/:id/payments/:paymentId` | `DeletePayment` | Remove a payment |
| GET | `/api/expenses/:id/payment-status` | `GetPaymentStatus` | Total owed vs paid on expense |
| GET | `/api/groups/:id/expense-payment-statuses` | `GetGroupExpensePaymentStatuses` | Batch: status for all expenses |

---

## 🔧 activity.go

### Endpoints

| Method | Endpoint | Function | Description |
|--------|----------|----------|-------------|
| GET | `/api/groups/:id/activities` | `GetActivities` | Activity feed for a group (last 50) |

---

## 🔧 suggestions.go

### Endpoints

| Method | Endpoint | Function | Description |
|--------|----------|----------|-------------|
| POST | `/api/suggestions` | `CreateSuggestion` | Submit a feature suggestion (max 20 per user) |
| GET | `/api/suggestions` | `GetSuggestions` | List all suggestions with vote counts |
| DELETE | `/api/suggestions/:id` | `DeleteSuggestion` | Delete own suggestion |
| POST | `/api/suggestions/:id/vote` | `VoteSuggestion` | Upvote a suggestion (anonymous) |
| DELETE | `/api/suggestions/:id/vote` | `UnvoteSuggestion` | Remove your vote |

---

## 🔧 currency.go

### Handler Struct

```go
type CurrencyHandler struct {
    cache        map[string]*rateCache      // Live rates cache
    historyCache map[string]*historyCache   // Historical rates cache
    cacheMutex   sync.RWMutex               // Thread-safe access
}
```

### Cache Structs

| Struct | Fields | TTL |
|--------|--------|-----|
| `rateCache` | `rates map[string]float64`, `fetchedAt time.Time` | 1 hour |
| `historyCache` | `data []HistoricalRate`, `fetchedAt time.Time` | 24 hours |

### Endpoints

| Method | Endpoint | Function | Description |
|--------|----------|----------|-------------|
| GET | `/api/currency/rates` | `GetRates` | Get exchange rates for a base currency |
| GET | `/api/currency/convert` | `Convert` | Convert amount between currencies |
| GET | `/api/currency/history` | `GetHistory` | Historical rates for trend charts |

### External APIs Used

| API | URL | Purpose |
|-----|-----|---------|
| Open Exchange Rates | `open.er-api.com/v6/latest/{base}` | Live exchange rates (free tier) |
| Frankfurter | `api.frankfurter.app/{start}..{end}` | Historical rates (ECB data) |

---

## 🔄 Common Patterns

### Handler Constructor
```go
func NewUserHandler(database *sql.DB) *UserHandler {
    return &UserHandler{DB: database}
}
```

### Getting User ID from JWT (set by middleware)
```go
userID := c.GetInt("userID")  // Extracted from JWT by auth middleware
```

### Standard Response Format
```go
c.JSON(http.StatusOK, models.APIResponse{
    Success: true,
    Data:    result,
    Message: "Optional success message",
})
```

### Error Response Format
```go
c.JSON(http.StatusBadRequest, models.APIResponse{
    Success: false,
    Error:   "Description of what went wrong",
})
```

### Getting URL Parameters
```go
groupID, _ := strconv.Atoi(c.Param("id"))        // From URL path
amount := c.Query("amount")                       // From query string
base := c.DefaultQuery("base", "USD")            // With default
```
