# GopherDebt

A full-featured expense sharing and personal finance backend built with Go, Gin, and PostgreSQL. Deployed on Fly.io with Supabase as the managed database.

## Features

- **Group Expenses** — Create groups, add members, split expenses (equal, exact, percentage)
- **Settlements & Balances** — Track who owes whom, record payments, view debt overview
- **Expense Payments** — Partial repayments toward specific expenses
- **GopherStash** — Personal expense tracker with categories and spending summaries
- **GoWish** — Wishlist of things to buy (name, why, link, cost), personal or in shared wish groups (separate from debt groups)
- **GoCrumbs** — Monthly "what's left" calculator: German gross-to-net (Lohnsteuer, Soli, church tax, social insurance; employees and Werkstudenten) minus rent, savings and subscriptions. One saved profile per user, no AI
- **Receipt Scanning** — AI-powered receipt parsing via OpenAI (default model `gpt-6-luna`)
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
| `JWT_SECRET` | Yes | — | Secret key for JWT token signing (server refuses to start without it) |
| `PORT` | No | `8080` | Server port |
| `OPENAI_API_KEY` | No | — | OpenAI API key for receipt scanning (without it, the app uses on-device OCR) |
| `OPENAI_RECEIPT_MODEL` | No | `gpt-6-luna` | OpenAI model that reads receipts and, as a fallback, product pages for wish links |
| `AI_DAILY_RECEIPT_SCANS` | No | `20` | AI receipt scans per user per day (over it: on-device OCR); `0` turns AI scanning off |
| `AI_DAILY_WISH_LOOKUPS` | No | `50` | AI calls for wish links per user per day (page reading + name shortening; shop data is free and doesn't count). `0` turns the wish AI off (shop data only, long names cut at a word); fly.toml sets `0` for now |

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
| DELETE | `/api/users/:id` | Delete a user (founder only; anonymized as "Deleted user" so balances and history stay correct) |
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
| DELETE | `/api/groups/:id/expenses` | Clear all group expenses and settlements (every balance back to 0; group and members kept) |

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
| POST | `/api/receipt/scan` | Upload receipt image (max 14 MB) → AI extracts items & total |
| GET | `/api/receipt/config` | Whether AI scanning is available (`OPENAI_API_KEY` set) |

### GopherStash — Personal Expense Tracker (Protected)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/stash` | Get all personal expenses |
| POST | `/api/stash` | Add a personal expense |
| PUT | `/api/stash/:id` | Update a personal expense |
| DELETE | `/api/stash/:id` | Delete a personal expense |
| GET | `/api/stash/summary` | Get total spent + category breakdown |
| DELETE | `/api/stash` | Clear all personal expenses |

### GoWish — Personal Wishlist (Protected)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/wish` | Get all wishlist items (newest first) |
| POST | `/api/wish/lookup` | Read a product link (`{"url"}`) → `{status, name, price, currency, source}` to fill in the Add wish form |
| POST | `/api/wish` | Add a wishlist item |
| PUT | `/api/wish/:id` | Update a wishlist item |
| DELETE | `/api/wish/:id` | Delete a wishlist item |

### GoWish — Shared Wish Groups (Protected, members only)

Separate from debt groups. Each member has their own section of wishes in the group (the wishes they added). Every member can rename the group, add/remove members, and edit/delete any wish. When a member leaves or is removed, their wishes in that group are deleted; when the last member leaves, the group is deleted. Non-members get `404`.

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/wish-lists` | Groups I'm a member of (with members, item count, total cost) |
| POST | `/api/wish-lists` | Create a group (`{"name", "description", "emoji", "member_ids"}`, same options as a debt group); creator is always a member |
| GET | `/api/wish-lists/:id` | Get a group with its members |
| PUT | `/api/wish-lists/:id` | Edit a group's name, description and icon |
| POST | `/api/wish-lists/:id/members` | Add a member (`{"user_id"}`) |
| DELETE | `/api/wish-lists/:id/members/:userId` | Remove a member (and their wishes in the group), or leave (own ID); returns `{"list_deleted"}` |
| GET | `/api/wish-lists/:id/items` | Wishes in the group (each with `added_by`) |
| POST | `/api/wish-lists/:id/items` | Add a wish (same body as `/api/wish`) |
| PUT | `/api/wish-lists/:id/items/:itemId` | Edit a wish |
| DELETE | `/api/wish-lists/:id/items/:itemId` | Delete a wish |

### GoCrumbs — Monthly Budget Calculator (Protected)

One saved profile per user. The profile stores inputs only; net pay and "left" are recalculated on every request, using the latest supported tax year (shown as `tax_year` in the result).

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/gocrumbs` | Get my saved profile (`data` is absent until the first save) |
| PUT | `/api/gocrumbs` | Create or replace my profile (must be calculable) and return it |
| POST | `/api/gocrumbs/calculate` | Calculate net pay and what's left for the given inputs, without saving |

### Access Control (Protected, Founder Only)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/whitelist` | Get whitelisted emails |
| POST | `/api/whitelist` | Add email to whitelist |
| DELETE | `/api/whitelist/:id` | Remove from whitelist |
| GET | `/api/blacklist` | Get blacklisted emails |
| POST | `/api/blacklist` | Add email to blacklist |
| DELETE | `/api/blacklist/:id` | Remove from blacklist |
| POST | `/api/admin/reset-finances` | Clear all expenses, payments and settlements (keeps users and groups) |

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

### Add Wishlist Item (GoWish)
```json
POST /api/wish
Authorization: Bearer <token>
{
  "name": "Mechanical keyboard",
  "why": "Typing on the laptop keyboard hurts",
  "link": "https://example.com/keyboard",
  "cost": 149.99
}
```
Only `name` is required (max 64 chars). `why` (max 128 chars), `link` and `cost` are optional; `link` must be an `http(s)://` URL and `cost` must be > 0 when set.

### Save GoCrumbs Budget
```json
PUT /api/gocrumbs
Authorization: Bearer <token>
{
  "work_type": "fte",
  "gross_amount": 4000,
  "gross_period": "month",
  "side_gross": 0,
  "steuerklasse": 1,
  "church_tax": false,
  "bundesland": "NW",
  "children": 0,
  "under_23": false,
  "zusatzbeitrag": 2.9,
  "rent": 900,
  "savings": 300,
  "subscriptions": [{ "name": "Netflix", "amount": 12.99, "category": "media" }],
  "custom_expenses": [{ "name": "Phone", "amount": 20 }],
  "extra_pay": [
    { "name": "On-call", "amount": 500 },
    { "name": "Sunday surcharge", "amount": 150, "tax_free": true }
  ]
}
```
`work_type` is `fte` or `werkstudent` (a Werkstudent can't have `side_gross`). `gross_period` is `month` or `year`; every other amount is EUR per month. `bundesland` is the two-letter code, `zusatzbeitrag` the health insurer's rate in % (0-10). At most 50 expense lines in total and 20 `extra_pay` lines, names up to 60 characters. `extra_pay` is flat monthly pay on top of the salary (e.g. on-call): taxable and subject to social insurance like salary, or paid out in full when `tax_free` is true. `POST /api/gocrumbs/calculate` takes the same body and returns the breakdown: `gross` (incl. tax-free extras), `extra_taxable`, `extra_free`, `rv`, `av`, `kv`, `pv`, `lohnsteuer`, `soli`, `kirchensteuer`, `net`, `expenses`, `left`, the rates and `notes`.

### Scan Receipt
```
POST /api/receipt/scan
Authorization: Bearer <token>
Content-Type: multipart/form-data

image: <receipt image file>
```
Max 14 MB (413 if larger), to keep server memory bounded. JPEG, PNG, WEBP or GIF only (415 otherwise). At most 2 scans run at once (429 if busy).

## Project Structure

```
├── main.go                # Entry point, router, middleware setup
├── Dockerfile             # Multi-stage Docker build
├── fly.toml               # Fly.io deployment config
├── db/
│   ├── migrations.go      # Database schema & migrations
│   ├── security.go        # Supabase REST API lockdown + case-insensitive unique emails
│   ├── users.go           # User CRUD
│   ├── groups.go          # Group CRUD
│   ├── expenses.go        # Expense CRUD
│   ├── settlements.go     # Settlements + balance calculation
│   ├── expense_payments.go # Partial expense payments
│   ├── activity.go        # Activity logging
│   ├── suggestions.go     # Suggestion box
│   ├── stash.go           # GopherStash personal expenses
│   ├── wish.go            # GoWish personal wishlist
│   ├── wish_lists.go      # GoWish shared wish groups
│   ├── gocrumbs.go        # GoCrumbs budget profile, expense + extra pay lines
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
│   ├── wish.go            # GoWish personal endpoints
│   ├── wish_lookup.go     # Product link → name/price (shop data first, OpenAI fallback, SSRF-safe)
│   ├── ai_guard.go        # AI guardrails: daily per-user limits, token usage log, lookup cache
│   ├── wish_lists.go      # GoWish shared group endpoints
│   ├── gocrumbs.go        # GoCrumbs endpoints (profile + calculate)
│   ├── access_control.go  # Whitelist/blacklist (founder only)
│   ├── receipt.go         # AI receipt scanning via OpenAI
│   ├── handlers_test.go   # Handler tests
│   ├── stash_test.go      # GopherStash tests
│   ├── wish_test.go       # GoWish tests
│   ├── wish_lists_test.go # GoWish shared group tests
│   ├── gocrumbs_test.go   # GoCrumbs validation + calculate endpoint tests
│   └── receipt_test.go    # Receipt scan limits + OpenAI request/errors (fake server)
├── gocrumbs/              # GoCrumbs engine: German gross-to-net, pure functions (see gocrumbs/gocrumbs_readme.md)
│   ├── params.go          # Tax-year constants (2026)
│   ├── tariff.go          # § 32a EStG income tax tariff
│   ├── lohnsteuer.go      # Port of the BMF Programmablaufplan (Lohnsteuer, Soli, Kirchensteuer base)
│   ├── social.go          # Employee social insurance, Midijob, Werkstudent
│   ├── calc.go            # Calculate(Input) → Result
│   └── testdata/          # The BMF's official check tables, used as test reference
├── middleware/
│   ├── auth.go            # JWT authentication
│   ├── active_user.go     # Rejects deleted/blacklisted accounts, outdated tokens
│   ├── rate_limit.go      # Per-IP rate limiting
│   └── auth_test.go       # Auth middleware tests
└── models/
    └── models.go          # All data structures & request/response models
```

## Testing

```bash
go test ./...
```

Over 100 tests covering authentication, group operations, expense validation, GopherStash, GoWish (personal + shared) and GoCrumbs validation, the GoCrumbs tax engine (checked against the BMF's official reference tables) and middleware.

## Deployment

Deployed on **Fly.io** with a multi-stage Docker build. Database hosted on **Supabase** (PostgreSQL).

```bash
fly deploy
```
