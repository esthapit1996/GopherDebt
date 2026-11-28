# 🏗️ GopherDebt Backend Architecture

This document explains how the Go backend is structured and how all the pieces fit together.

---

## 📚 Libraries & Dependencies

| Library | Import | Purpose |
|---------|--------|---------|
| **Gin** | `github.com/gin-gonic/gin` | Web framework (routing, middleware, JSON handling) |
| **lib/pq** | `github.com/lib/pq` | PostgreSQL driver for database/sql |
| **jwt-go** | `github.com/golang-jwt/jwt/v5` | JSON Web Token creation and validation |
| **bcrypt** | `golang.org/x/crypto/bcrypt` | Password hashing |
| **database/sql** | stdlib | Database interface |
| **sync** | stdlib | Thread-safe caching (rate limiter, currency cache) |
| **encoding/json** | stdlib | Parse JSON from external APIs |
| **encoding/base64** | stdlib | Encode receipt images for the OpenAI API |

---

## 📁 Project Structure

```
GopherDebt/
├── main.go              # Entry point - sets up everything
├── go.mod               # Go module dependencies
├── gopherdebt           # Compiled binary
├── Dockerfile           # Multi-stage Docker build for Fly.io
├── fly.toml             # Fly.io deployment config
├── db/                  # Database layer
│   ├── db_readme.md     # 📖 Documentation
│   ├── users.go         # User CRUD
│   ├── groups.go        # Group CRUD  
│   ├── expenses.go      # Expense CRUD
│   ├── settlements.go   # Settlements + balance calculation
│   ├── expense_payments.go  # Partial expense payments
│   ├── activity.go      # Activity logging
│   ├── suggestions.go   # Suggestion box
│   ├── stash.go         # GopherStash personal expenses
│   ├── wish.go          # GoWish personal wishlist
│   ├── wish_lists.go    # GoWish shared wish groups
│   ├── gocrumbs.go      # GoCrumbs single profile (deprecated), expense + extra pay lines
│   ├── gocrumbs_pages.go # GoCrumbs pages (15 per work type) + one-time copy of old profiles
│   ├── access_control.go # Email whitelist/blacklist
│   ├── retry.go         # Generic retry with backoff
│   ├── migrations.go    # Schema creation
│   └── security.go      # Supabase REST API lockdown, unique emails
├── handlers/            # HTTP endpoint controllers
│   ├── handlers_readme.md   # 📖 Documentation
│   ├── handlers_test.go # Handler tests (+ stash_test.go, wish_test.go, wish_lists_test.go, gocrumbs_test.go, gocrumbs_pages_test.go)
│   ├── users.go         # Auth & profile endpoints
│   ├── groups.go        # Group endpoints
│   ├── expenses.go      # Expense endpoints
│   ├── settlements.go   # Settlement & balance endpoints
│   ├── expense_payments.go  # Payment endpoints
│   ├── activity.go      # Activity feed endpoints
│   ├── suggestions.go   # Suggestion box endpoints
│   ├── currency.go      # Currency conversion endpoints
│   ├── stash.go         # GopherStash endpoints
│   ├── wish.go          # GoWish personal endpoints
│   ├── wish_lookup.go   # Product link lookup for wishes (SSRF-safe fetch, OpenAI fallback)
│   ├── ai_guard.go      # AI guardrails (daily limits, usage log, cache)
│   ├── wish_lists.go    # GoWish shared group endpoints
│   ├── gocrumbs.go      # GoCrumbs endpoints (calculate + deprecated single profile)
│   ├── gocrumbs_pages.go # GoCrumbs page endpoints
│   ├── access_control.go # Whitelist/blacklist (founder only)
│   └── receipt.go       # AI receipt scanning (OpenAI, key server-side)
├── gocrumbs/            # GoCrumbs engine (pure functions, no DB or HTTP)
│   ├── gocrumbs_readme.md # 📖 Documentation
│   ├── params.go        # Tax-year constants
│   ├── tariff.go        # § 32a EStG income tax tariff
│   ├── lohnsteuer.go    # Port of the BMF Programmablaufplan
│   ├── social.go        # Social insurance, Midijob, Werkstudent
│   └── calc.go          # Calculate(Input) → Result
├── middleware/          # Request processing
│   ├── middleware_readme.md # 📖 Documentation
│   ├── auth.go          # JWT authentication
│   ├── active_user.go   # Rejects deleted/blacklisted accounts, outdated tokens
│   ├── auth_test.go     # Auth middleware tests
│   └── rate_limit.go    # Per-IP rate limiting
└── models/              # Data structures
    ├── models_readme.md # 📖 Documentation
    └── models.go        # All model definitions
```

---

## 🔄 Request Flow

```
┌─────────┐     ┌──────────┐     ┌────────────┐     ┌────────────┐     ┌─────────┐     ┌──────────┐
│ Frontend │ ──► │   Gin    │ ──► │   Rate     │ ──► │ Middleware │ ──► │ Handler │ ──► │ Database │
│ (React)  │ ◄── │  Router  │ ◄── │  Limiter   │ ◄── │   (Auth)   │ ◄── │         │ ◄── │  Layer   │
└─────────┘     └──────────┘     └────────────┘     └────────────┘     └─────────┘     └──────────┘
    JSON            Routes          Per-IP              JWT              Business       SQL Queries
   Request         Matching        Throttle           Validation          Logic        + Retry Logic
```

---

## 🔧 main.go Explained

### 1. Database Connection
```go
connStr := fmt.Sprintf("host=... password=%s dbname=postgres", dbPassword)
database, err := sql.Open("postgres", connStr)
```
Connects to Supabase PostgreSQL using connection string.

### 2. Run Migrations
```go
db.RunMigrations(database)
```
Creates all tables if they don't exist (safe to run multiple times).

### 3. Initialize Handlers
```go
userHandler := handlers.NewUserHandler(database)
groupHandler := handlers.NewGroupHandler(database)
stashHandler := handlers.NewStashHandler(database)
wishHandler := handlers.NewWishHandler(database)
wishListHandler := handlers.NewWishListHandler(database)
goCrumbsHandler := handlers.NewGoCrumbsHandler(database)
receiptHandler := handlers.NewReceiptHandler()
// ... each handler gets the database connection
```
Handlers are structs that hold the DB connection and have methods for each endpoint.

### 4. Setup Router
```go
r := gin.Default()
```
Creates a Gin router with default middleware (logging, recovery).

### 5. CORS & Cache Prevention Middleware
```go
r.Use(func(c *gin.Context) {
    c.Header("Access-Control-Allow-Origin", "*")
    c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
    // ... other headers
})
```
Allows frontend (on different domain) to make requests and prevents stale cached responses.

### 6. Rate Limiting
```go
authLimiter := middleware.NewRateLimiter(10, time.Minute)  // 10 req/min for auth
apiLimiter := middleware.NewRateLimiter(100, time.Minute)   // 100 req/min for API
```
Per-IP rate limiting with automatic cleanup of stale entries.

### 6. Public Routes
```go
r.POST("/api/register", userHandler.Register)
r.POST("/api/login", userHandler.Login)
```
No authentication required.

### 7. Protected Routes
```go
api := r.Group("/api")
api.Use(middleware.AuthMiddleware())
api.Use(apiLimiter.Middleware())
api.Use(middleware.ActiveUser(database)) // deleted / blacklisted accounts, outdated tokens → 401
{
    api.GET("/profile", userHandler.GetProfile)
    // ... all protected routes
}
```
All routes in this group require valid JWT token.

### 8. Start Server
```go
r.Run(":8080")
```
Starts listening for HTTP requests.

---

## 🗄️ Database Schema

```
┌─────────────┐       ┌─────────────┐       ┌──────────────┐
│   users     │       │   groups    │       │group_members │
├─────────────┤       ├─────────────┤       ├──────────────┤
│ id (PK)     │◄──┐   │ id (PK)     │◄──────│ group_id(FK) │
│ email       │   │   │ name        │       │ user_id (FK) │──┐
│ password    │   │   │ description │       │ joined_at    │  │
│ name        │   │   │ emoji       │       └──────────────┘  │
│ theme       │   │   │ created_by  │───┐                     │
└─────────────┘   │   └─────────────┘   │                     │
      ▲           │          ▲          │                     │
      │           │          │          └─────────────────────┤
      │           │          │                                │
┌─────┴───────┐   │   ┌──────┴──────┐                        │
│  expenses   │   │   │ settlements │                        │
├─────────────┤   │   ├─────────────┤                        │
│ id (PK)     │   │   │ id (PK)     │                        │
│ group_id    │───┘   │ group_id    │                        │
│ paid_by(FK) │───────│ paid_by(FK) │◄───────────────────────┘
│ amount      │       │ paid_to(FK) │
│ description │       │ amount      │
│ split_type  │       └─────────────┘
└─────────────┘
      │
      ▼
┌─────────────────┐    ┌───────────────────┐
│ expense_splits  │    │ expense_payments  │
├─────────────────┤    ├───────────────────┤
│ id (PK)         │    │ id (PK)           │
│ expense_id (FK) │    │ expense_id (FK)   │
│ user_id (FK)    │    │ paid_by (FK)      │
│ amount          │    │ amount            │
└─────────────────┘    │ note              │
                       └───────────────────┘

┌──────────────────┐   ┌──────────────────┐   ┌──────────────────┐
│ stash_expenses   │   │ email_whitelist  │   │ email_blacklist  │
├──────────────────┤   ├──────────────────┤   ├──────────────────┤
│ id (PK)          │   │ id (PK)          │   │ id (PK)          │
│ user_id (FK)     │   │ email            │   │ email            │
│ amount           │   │ added_by (FK)    │   │ reason           │
│ description      │   │ created_at       │   │ added_by (FK)    │
│ category         │   └──────────────────┘   │ created_at       │
│ created_at       │                          └──────────────────┘
└──────────────────┘

┌──────────────────┐   ┌──────────────────┐   ┌───────────────────┐
│ wish_items       │   │ wish_lists       │   │ wish_list_members │
├──────────────────┤   ├──────────────────┤   ├───────────────────┤
│ id (PK)          │   │ id (PK)          │   │ id (PK)           │
│ user_id (FK)     │   │ name             │   │ list_id (FK)      │
│ list_id (FK)     │   │ description      │   │ user_id (FK)      │
│ name             │   │ emoji            │   │ joined_at         │
│ why              │   │ created_by (FK)  │   └───────────────────┘
│ link             │   │ created_at       │
│                  │   └──────────────────┘
│ cost (nullable)  │   wish_items.list_id NULL = personal wish (owned by user_id);
│ created_at       │   set = shared wish (user_id = who added it)
└──────────────────┘

┌───────────────────┐   ┌───────────────────┐   ┌───────────────────┐
│ gocrumbs_profiles │   │ gocrumbs_expenses │   │ gocrumbs_extra_pay│
├───────────────────┤   ├───────────────────┤   ├───────────────────┤
│ user_id (PK, FK)  │   │ id (PK)           │   │ id (PK)           │
│ work_type         │   │ user_id (FK)      │   │ user_id (FK)      │
│ gross_amount      │   │ kind              │   │ name              │
│ gross_period      │   │ name              │   │ amount            │
│ side_gross        │   │ amount            │   │ tax_free          │
│ steuerklasse      │   │ category          │   │ position          │
│ church_tax        │   │ position          │   └───────────────────┘
│ bundesland        │   └───────────────────┘   flat pay on top of the
│ children          │   kind = subscription     salary, e.g. on-call
│ under_23          │          | custom
│ zusatzbeitrag     │
│ rent, savings     │   Inputs only: net pay and "left"
│ ws_hourly_wage    │   are always recalculated
│ ws_hours          │
│ ws_hours_period   │   Werkstudent income has its own
│ updated_at        │   columns, apart from the gross
└───────────────────┘   fields of an employee

Deprecated: the three tables above hold the old single profile. Their data was copied once into
the page tables below (flag in migration_flags) and the page code never writes them.

┌───────────────────┐   ┌────────────────────────┐   ┌─────────────────────────┐
│ gocrumbs_pages    │   │ gocrumbs_page_expenses │   │ gocrumbs_page_extra_pay │
├───────────────────┤   ├────────────────────────┤   ├─────────────────────────┤
│ id (PK)           │   │ id (PK)                │   │ id (PK)                 │
│ user_id (FK)      │   │ page_id (FK, cascade)  │   │ page_id (FK, cascade)   │
│ name              │   │ kind                   │   │ name                    │
│ work_type (fixed) │   │ name                   │   │ amount                  │
│ ...same inputs as │   │ amount                 │   │ tax_free                │
│ gocrumbs_profiles │   │ category               │   │ position                │
│ created_at        │   │ position               │   └─────────────────────────┘
│ updated_at        │   └────────────────────────┘
└───────────────────┘   Up to 15 pages per user and work type;
                        name unique per user + work type (ignoring case)
```

---

## 🔐 Authentication Flow

```
1. User registers → password hashed with bcrypt → stored in DB
2. User logs in → bcrypt.Compare → JWT token generated
3. Frontend stores token in localStorage
4. Every API request includes: "Authorization: Bearer <token>"
5. AuthMiddleware validates token → extracts userID (+ token version) → sets in context
6. ActiveUser checks the account still exists, isn't deleted or blacklisted, and the token
   version matches (a password change bumps it, logging out other devices)
7. Handler reads userID from context: c.GetInt("userID")
```

---

## 💰 Balance Calculation (The Core Algorithm)

```
For each user in group:
    balance = 0

For each expense:
    payer.balance += expense.amount  (they paid, they're owed)
    For each split:
        owingUser.balance -= split.amount  (they owe)

For each settlement:
    payer.balance += amount   (they paid back)
    receiver.balance -= amount (they received payment)

For each expense_payment:
    payer.balance += amount
    expenseOwner.balance -= amount

Result: positive balance = others owe you
        negative balance = you owe others
```

---

## 🌐 API Response Format

All endpoints return this structure:

```json
{
    "success": true,
    "message": "Optional success message",
    "data": { ... },  // The actual response data
    "error": "Error message if success is false"
}
```

---

## 📖 Per-Folder Documentation

Each folder has its own detailed README:

| File | What it documents |
|------|-------------------|
| [db/db_readme.md](db/db_readme.md) | All database functions |
| [handlers/handlers_readme.md](handlers/handlers_readme.md) | All API endpoints |
| [middleware/middleware_readme.md](middleware/middleware_readme.md) | Authentication middleware |
| [models/models_readme.md](models/models_readme.md) | All data structures |

---

## 🚀 Running the Server

```bash
# Set environment variables
export DB_PASSWORD="your_password"
export JWT_SECRET="your_secret"

# Run directly
go run main.go

# Or build and run
go build -o gopherdebt
./gopherdebt
```

Server runs on `http://localhost:8080`
