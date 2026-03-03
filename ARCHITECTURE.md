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
| **encoding/base64** | stdlib | Encode receipt images for Gemini API |

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
│   ├── access_control.go # Email whitelist/blacklist
│   ├── retry.go         # Generic retry with backoff
│   └── migrations.go    # Schema creation
├── handlers/            # HTTP endpoint controllers
│   ├── handlers_readme.md   # 📖 Documentation
│   ├── handlers_test.go # 31 handler tests
│   ├── users.go         # Auth & profile endpoints
│   ├── groups.go        # Group endpoints
│   ├── expenses.go      # Expense endpoints
│   ├── settlements.go   # Settlement & balance endpoints
│   ├── expense_payments.go  # Payment endpoints
│   ├── activity.go      # Activity feed endpoints
│   ├── suggestions.go   # Suggestion box endpoints
│   ├── currency.go      # Currency conversion endpoints
│   ├── stash.go         # GopherStash endpoints
│   ├── access_control.go # Whitelist/blacklist (founder only)
│   └── receipt.go       # AI receipt scanning (Gemini proxy)
├── middleware/          # Request processing
│   ├── middleware_readme.md # 📖 Documentation
│   ├── auth.go          # JWT authentication
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
```

---

## 🔐 Authentication Flow

```
1. User registers → password hashed with bcrypt → stored in DB
2. User logs in → bcrypt.Compare → JWT token generated
3. Frontend stores token in localStorage
4. Every API request includes: "Authorization: Bearer <token>"
5. AuthMiddleware validates token → extracts userID → sets in context
6. Handler reads userID from context: c.GetInt("userID")
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
