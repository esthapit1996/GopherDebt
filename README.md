# GopherDebt

A full-featured expense sharing and personal finance backend built with Go, Gin, and PostgreSQL. Deployed on Fly.io with Supabase as the managed database.

## Features

- **Group Expenses** — Create groups, add members, split expenses (equal, exact, percentage)
- **Settlements & Balances** — Track who owes whom, record payments, view debt overview
- **Expense Payments** — Partial repayments toward specific expenses
- **GopherStash** — Personal expense tracker with categories and spending summaries
- **Receipt Scanning** — AI-powered receipt parsing via Google Gemini
- **Currency Conversion** — Live exchange rates and historical trend data
- **Suggestion Box** — Feature suggestions with voting, comments, and status tracking
- **Access Control** — Email whitelist/blacklist managed by the founder
- **Rate Limiting** — Per-IP rate limiting (strict for auth, normal for API)
- **Activity Feed** — Detailed group activity history
- **i18n Ready** — Backend supports the multilingual frontend (EN + IT)

## Setup

```bash
# Install dependencies
go mod tidy

# Build
go build -o gopherdebt .

# Run
./gopherdebt
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | No | — | Full PostgreSQL connection string (overrides `DB_PASSWORD`) |
| `DB_PASSWORD` | Yes* | — | Supabase DB password (*required if `DATABASE_URL` is not set) |
| `JWT_SECRET` | No | `"your-secret-key-change-in-production"` | Secret key for JWT token signing |
| `PORT` | No | `8080` | Server port |
| `GEMINI_API_KEY` | No | — | Google Gemini API key for receipt scanning |

## API Endpoints

### Public

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/` | API info & version |
| GET | `/health` | Health check (pings DB) |
| POST | `/api/register` | Register a new user (rate limited: 10/min) |
| POST | `/api/login` | Login and get JWT token (rate limited: 10/min) |

### Users (Protected)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/profile` | Get current user profile |
| GET | `/api/users` | Get all users |
| DELETE | `/api/users/:id` | Delete a user |
| GET | `/api/debt-overview` | Get debt overview across all groups |
| GET | `/api/debt-overview/:userId` | Get debt details with a specific user |
| GET | `/api/payment-history` | Get payment history |
| DELETE | `/api/payment-history` | Clear payment history |
| PUT | `/api/profile/theme` | Update theme preference |
| PUT | `/api/profile/avatar` | Update avatar |
| PUT | `/api/profile/language` | Update language preference |
| PUT | `/api/profile/password` | Change password |

### Groups (Protected)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/groups` | Create a new group |
| GET | `/api/groups` | Get user's groups (with balances) |
| GET | `/api/groups/:id` | Get group details |
| PUT | `/api/groups/:id` | Update a group |
| DELETE | `/api/groups/:id` | Delete a group (creator only) |
| POST | `/api/groups/:id/members` | Add a member to group |
| DELETE | `/api/groups/:id/members/:memberID` | Remove a member |

### Expenses (Protected)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/groups/:id/expenses` | Create expense (equal/exact/percentage split) |
| GET | `/api/groups/:id/expenses` | Get group expenses |
| GET | `/api/groups/:id/expenses/unpaid` | Get unpaid expenses |
| GET | `/api/groups/:id/expenses/:expenseID` | Get expense details |
| DELETE | `/api/groups/:id/expenses/:expenseID` | Delete expense (payer only) |
| DELETE | `/api/groups/:id/expenses` | Clear all group expenses |

### Expense Payments (Protected)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/expenses/:id/payments` | Record partial payment on expense |
| GET | `/api/expenses/:id/payments` | List payments for an expense |
| DELETE | `/api/payments/:paymentId` | Delete a payment |
| GET | `/api/groups/:id/expense-payment-statuses` | Batch: payment status for all group expenses |

### Settlements & Balances (Protected)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/groups/:id/settlements` | Record a settlement |
| GET | `/api/groups/:id/settlements` | Get group settlements |
| DELETE | `/api/groups/:id/settlements/:settlementID` | Delete a settlement |
| GET | `/api/groups/:id/balances` | Get who owes whom |
| GET | `/api/groups/:id/my-balance` | Get your balance |

### Activity (Protected)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/groups/:id/activities` | Activity feed for a group (last 50) |

### Suggestions (Protected)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/suggestions` | List all suggestions with vote counts |
| POST | `/api/suggestions` | Submit a feature suggestion |
| PUT | `/api/suggestions/:id` | Edit own suggestion |
| DELETE | `/api/suggestions/:id` | Delete own suggestion |
| POST | `/api/suggestions/:id/vote` | Upvote a suggestion |
| DELETE | `/api/suggestions/:id/vote` | Remove vote |
| GET | `/api/suggestions/:id/voters` | List voters |
| PUT | `/api/suggestions/:id/status` | Update suggestion status (founder only) |
| GET | `/api/suggestions/:id/comments` | Get comments |
| POST | `/api/suggestions/:id/comments` | Add comment |
| PUT | `/api/suggestions/:id/comments/:commentId` | Edit comment |
| DELETE | `/api/suggestions/:id/comments/:commentId` | Delete comment |

### Currency (Protected)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/currency/rates` | Get exchange rates for a base currency |
| GET | `/api/currency/convert` | Convert amount between currencies |
| GET | `/api/currency/history` | Historical rates for trend charts |

### Receipt Scanning (Protected)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/receipt/scan` | Upload receipt image → AI extracts items & total |

### GopherStash — Personal Expense Tracker (Protected)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/stash` | Get all personal expenses |
| POST | `/api/stash` | Add a personal expense |
| DELETE | `/api/stash/:id` | Delete a personal expense |
| GET | `/api/stash/summary` | Get total spent + category breakdown |
| DELETE | `/api/stash` | Clear all personal expenses |

### Access Control (Protected, Founder Only)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/whitelist` | Get whitelisted emails |
| POST | `/api/whitelist` | Add email to whitelist |
| DELETE | `/api/whitelist/:id` | Remove from whitelist |
| GET | `/api/blacklist` | Get blacklisted emails |
| POST | `/api/blacklist` | Add email to blacklist |
| DELETE | `/api/blacklist/:id` | Remove from blacklist |

## Request Examples

### Register
```json
POST /api/register
{
  "email": "user@example.com",
  "password": "password123",
  "name": "John Doe"
}
```

### Login
```json
POST /api/login
{
  "email": "user@example.com",
  "password": "password123"
}
```

### Create Group
```json
POST /api/groups
Authorization: Bearer <token>
{
  "name": "Roommates",
  "description": "Shared apartment expenses"
}
```

### Create Expense (Equal Split)
```json
POST /api/groups/1/expenses
Authorization: Bearer <token>
{
  "amount": 60.00,
  "description": "Dinner",
  "split_type": "equal"
}
```

### Create Expense (Exact Split)
```json
POST /api/groups/1/expenses
Authorization: Bearer <token>
{
  "amount": 100.00,
  "description": "Groceries",
  "split_type": "exact",
  "split_with": [
    {"user_id": 1, "amount": 50.00},
    {"user_id": 2, "amount": 30.00},
    {"user_id": 3, "amount": 20.00}
  ]
}
```

### Create Expense (Percentage Split)
```json
POST /api/groups/1/expenses
Authorization: Bearer <token>
{
  "amount": 100.00,
  "description": "Utilities",
  "split_type": "percentage",
  "split_with": [
    {"user_id": 1, "amount": 50},
    {"user_id": 2, "amount": 30},
    {"user_id": 3, "amount": 20}
  ]
}
```

### Record Settlement
```json
POST /api/groups/1/settlements
Authorization: Bearer <token>
{
  "paid_to": 2,
  "amount": 25.00
}
```

### Add Personal Expense (GopherStash)
```json
POST /api/stash
Authorization: Bearer <token>
{
  "amount": 12.50,
  "description": "Coffee beans",
  "category": "food"
}
```

### Scan Receipt
```
POST /api/receipt/scan
Authorization: Bearer <token>
Content-Type: multipart/form-data

image: <receipt image file>
```

## Project Structure

```
├── main.go                # Entry point, router, middleware setup
├── Dockerfile             # Multi-stage Docker build
├── fly.toml               # Fly.io deployment config
├── db/
│   ├── migrations.go      # Database schema & migrations
│   ├── users.go           # User CRUD
│   ├── groups.go          # Group CRUD
│   ├── expenses.go        # Expense CRUD
│   ├── settlements.go     # Settlements + balance calculation
│   ├── expense_payments.go # Partial expense payments
│   ├── activity.go        # Activity logging
│   ├── suggestions.go     # Suggestion box
│   ├── stash.go           # GopherStash personal expenses
│   ├── access_control.go  # Email whitelist/blacklist
│   └── retry.go           # Generic retry with backoff for transient DB errors
├── handlers/
│   ├── users.go           # Auth & user profile endpoints
│   ├── groups.go          # Group endpoints
│   ├── expenses.go        # Expense endpoints
│   ├── settlements.go     # Settlement & balance endpoints
│   ├── expense_payments.go # Payment endpoints
│   ├── activity.go        # Activity feed endpoints
│   ├── suggestions.go     # Suggestion box endpoints
│   ├── currency.go        # Currency conversion (cached external APIs)
│   ├── stash.go           # GopherStash endpoints
│   ├── access_control.go  # Whitelist/blacklist (founder only)
│   ├── receipt.go         # AI receipt scanning via Gemini
│   └── handlers_test.go   # 31 handler tests
├── middleware/
│   ├── auth.go            # JWT authentication
│   ├── rate_limit.go      # Per-IP rate limiting
│   └── auth_test.go       # Auth middleware tests
└── models/
    └── models.go          # All data structures & request/response models
```

## Testing

```bash
go test ./...
```

31 tests covering authentication, group operations, expense validation, GopherStash validation, and middleware.

## Deployment

Deployed on **Fly.io** with a multi-stage Docker build. Database hosted on **Supabase** (PostgreSQL).

```bash
fly deploy
```
