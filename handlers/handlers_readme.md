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
| `users.go` | Auth & user profile | `/api/register`, `/api/login`, `/api/profile/*`, `/api/users/*` |
| `groups.go` | Group management | `/api/groups/*` |
| `expenses.go` | Expense CRUD | `/api/groups/:id/expenses/*` |
| `settlements.go` | Direct payments & balances | `/api/groups/:id/settle`, `/api/groups/:id/balances` |
| `expense_payments.go` | Partial expense payments | `/api/expenses/:id/payments/*` |
| `activity.go` | Activity feed | `/api/groups/:id/activities` |
| `suggestions.go` | Feature suggestions | `/api/suggestions/*` |
| `currency.go` | Currency conversion | `/api/currency/*` |
| `stash.go` | GopherStash personal expenses | `/api/stash/*` |
| `wish.go` | GoWish personal wishlist | `/api/wish/*` |
| `wish_lookup.go` | Product link → name/price for the Add wish form | `/api/wish/lookup` |
| `ai_guard.go` | AI guardrails shared by receipts and wish lookups | — |
| `wish_lists.go` | GoWish shared wish groups | `/api/wish-lists/*` |
| `access_control.go` | Email whitelist/blacklist | `/api/whitelist/*`, `/api/blacklist/*` |
| `receipt.go` | AI receipt scanning | `/api/receipt/scan` |
| `handlers_test.go`, `stash_test.go`, `wish_test.go`, `wish_lists_test.go`, `receipt_test.go` | Handler tests | — |

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
| POST | `/api/register` | `Register` | Creates new user (email trimmed + lowercased; checks blacklist, whitelist and case-insensitive duplicates; hashes password) |
| POST | `/api/login` | `Login` | Validates credentials (email matched ignoring case). A blacklisted email gets 403 "This account has been blocked" (checked after the password). Returns a JWT carrying the user's `token_version` |
| GET | `/api/profile` | `GetProfile` | Returns current user's profile |
| GET | `/api/users` | `GetAllUsers` | Returns all users except deleted ones (for member selection) |
| DELETE | `/api/users/:id` | `DeleteUser` | Founder only. **Anonymizes** the account (see below); 404 if already deleted; the founder can't be deleted |
| PUT | `/api/profile/theme` | `UpdateTheme` | Updates user's theme preference |
| PUT | `/api/profile/avatar` | `UpdateAvatar` | Updates user's avatar |
| PUT | `/api/profile/language` | `UpdateLanguage` | Updates user's language (`en` / `it`) |
| PUT | `/api/profile/password` | `ChangePassword` | Changes the password and bumps `token_version`, which logs out every other device; returns `{"token"}` for the current one |

### Deleting a user

Accounts are anonymized, not removed, so other people's balances and group history stay correct. The row is renamed "Deleted user", gets an unusable email (`deleted-<id>@deleted.invalid`) and password, and all its tokens are revoked. GopherStash, wishes, wish-group memberships and the whitelist entry are removed. The user leaves every debt group where they're settled. Where they still owe or are owed money they stay as "Deleted user", so the others can still record settling up. Groups they created and all money records are kept.

### Helper Functions

| Function | Parameters | Returns | Description |
|----------|------------|---------|-------------|
| `generateJWT` | `userID, tokenVersion int` | `string, error` | Creates a signed JWT (7-day expiry) with claims `user_id`, `tv` (token version), `exp`, `iat` |

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
| PUT | `/api/groups/:id/expenses/:expenseID` | `UpdateExpense` | Edits an expense and replaces its splits |
| GET | `/api/groups/:id/expenses` | `GetGroupExpenses` | Lists all expenses in group |
| GET | `/api/groups/:id/expenses/unpaid` | `GetUnpaidExpenses` | Expenses the caller still owes on |
| GET | `/api/groups/:id/expenses/:expenseID` | `GetExpense` | Gets single expense details |
| DELETE | `/api/groups/:id/expenses/:expenseID` | `DeleteExpense` | Removes an expense |
| PUT | `/api/groups/:id/expenses/:expenseID/paid` | `MarkExpensePaid` | Toggles the expense's paid flag |
| DELETE | `/api/groups/:id/expenses` | `ClearAllExpenses` | **Clear all**: deletes every expense (with splits and expense payments) **and every settlement** in the group in one transaction, so all balances end at 0. Group and members are kept. Any member can do it; the frontend asks for confirmation |

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
| POST | `/api/groups/:id/settlements` | `CreateSettlement` | Records direct payment between users |
| GET | `/api/groups/:id/settlements` | `GetGroupSettlements` | Lists all settlements in group |
| DELETE | `/api/groups/:id/settlements/:settlementID` | `DeleteSettlement` | Deletes a settlement in this group |
| GET | `/api/groups/:id/balances` | `GetGroupBalances` | **Key!** Who owes whom in the group |
| GET | `/api/groups/:id/my-balance` | `GetMyBalance` | Current user's balance in group |

Debt overview and payment history live on `UserHandler` (users.go):

| Method | Endpoint | Function | Description |
|--------|----------|----------|-------------|
| GET | `/api/debt-overview` | `GetDebtOverview` | User's net debts across ALL groups |
| GET | `/api/debt-overview/:userId` | `GetDebtDetails` | Breakdown of debts with one user |
| GET | `/api/payment-history` | `GetPaymentHistory` | User's payment history (entries after their last clear) |
| DELETE | `/api/payment-history` | `ClearPaymentHistory` | Hides the user's history up to now, for that user only; deletes nothing, so balances and the other party's history are unchanged |

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

---

## 🔧 stash.go

### Handler Struct

```go
type StashHandler struct {
    DB *sql.DB
}
```

### Endpoints

| Method | Endpoint | Function | Description |
|--------|----------|----------|-------------|
| GET | `/api/stash` | `GetStashExpenses` | Get all personal expenses (newest first) |
| POST | `/api/stash` | `CreateStashExpense` | Add personal expense (description defaults to category name if empty) |
| PUT | `/api/stash/:id` | `UpdateStashExpense` | Edit a personal expense (owner only) |
| DELETE | `/api/stash/:id` | `DeleteStashExpense` | Delete a personal expense (owner only) |
| GET | `/api/stash/summary` | `GetStashSummary` | Total spent + count + breakdown by category |
| DELETE | `/api/stash` | `ClearStashExpenses` | Clear all personal expenses |

### Default Description Logic

When description is empty:
- If category is provided → capitalize category name (e.g., `"food"` → `"Food"`)
- If no category → default to `"Expense"`

---

## 🔧 wish.go

### Handler Struct

```go
type WishHandler struct {
    DB *sql.DB
}
```

### Endpoints

| Method | Endpoint | Function | Description |
|--------|----------|----------|-------------|
| GET | `/api/wish` | `GetWishItems` | Get all wishlist items (newest first) |
| POST | `/api/wish` | `CreateWishItem` | Add a wishlist item |
| PUT | `/api/wish/:id` | `UpdateWishItem` | Edit a wishlist item (owner only) |
| DELETE | `/api/wish/:id` | `DeleteWishItem` | Delete a wishlist item (owner only) |

### Validation

- `name` is trimmed and required (max 64 characters)
- `why` is trimmed and optional (max 128 characters)
- `cost` is optional (`null` = no price); when set it must be > 0
- `link` is optional; if set it must be an absolute `http`/`https` URL (max 2048). Other schemes such as `javascript:` are rejected because the frontend renders the link as an `href`.

Personal endpoints only see wishes with `list_id IS NULL`, so shared wishes can't be read or changed through `/api/wish`.

---

## 🔧 wish_lookup.go

`POST /api/wish/lookup` with `{"url": "https://shop…/product"}` fills in the Add wish form. It always answers 200 with a `status`, except for an invalid or forbidden URL, which gets 400:

| `status` | Meaning |
|----------|---------|
| `ok` | `name`, `price`, `currency` (ISO 4217) found; `source` is `page` (shop data) or `ai` |
| `blocked` | The shop refused automated reading: 401/403/429/503/999, a captcha or bot-check page (also when served with 200, e.g. Amazon's "continue shopping" page), or a page whose only "name" is the shop's own name (`isSiteName`). Bot pages never go to the AI |
| `not_found` | The page loaded but has no product name or price |
| `unreachable` | Couldn't load it (timeout, 404, not HTML, refused address) |
| `limit` | Nothing could be found without the AI, and the user's daily AI allowance is used up |

An `ok` answer can carry `"ai_limit": true` when the AI would have been needed (usually for the price) but the allowance is used up.
It can also carry `"price_in_cart": true` when the shop says the price is only shown in the cart (Amazon for some brands, detected in DE/EN/IT/FR). There is no price on the page then, so the AI isn't asked for it.

**How it reads the page:**
1. The page's structured product data, which costs nothing: JSON-LD `Product` (also inside `@graph` or arrays; `Offer` / `AggregateOffer` / `priceSpecification`), `product:price` / `og:price` meta tags, and `itemprop="price"` microdata.
   **Name:** the first candidate that isn't just the shop's own name, in this order: JSON-LD name, `og:title`, `id="productTitle"` (Amazon), each `<h1>`, then the page `<title>` / `<meta name="title">` with shop-name parts removed ("Amazon.de: X : Amazon.de: Küche" → "X"). The shop's name is checked against both the pasted host and the host after redirects (short links like `amzn.eu` → `amazon.de`).
2. Only if the name or price is missing (and the page isn't a bot page): a text excerpt of up to 12k characters goes to OpenAI with a strict `{name, price, currency}` schema. The excerpt leads with the title, product title, description, headings and up to 8 cleaned "price-looking" texts (elements whose class or id mentions "price"; doubled texts like "119,95€119,95€" halved, fragments and repeats dropped), then the visible text. Scripts, styles, `nav` / `header` / `footer` / `aside` and recommendation carousels (`carousel`, `p13n`, `similar`, `sponsored`…) are left out, so other products' prices never reach the AI. Amazon's buy-box price (`priceToPay`, `corePrice…`, `apex_desktop`) is read directly when the page shows one. Each AI answer is logged (`LookupWish: AI answer for …`). It uses the same model as receipts and costs about $0.0005 per lookup with `gpt-6-luna`. The prompt treats the page text as data, not instructions. If the page's name is longer than the 64-character field, the AI's shorter name is used.

**Long names:** any name over 64 characters (Amazon titles, for example) is shortened by one tiny extra AI call (`shortenName`, only the name is sent, about 100 tokens). It keeps the brand, product type and key details (model, size, capacity, colour) and drops marketing text and feature lists, in the original language. Without AI, or if the answer is still too long, the name is cut at a word boundary (`cutAtWord`).

**Security (SSRF):** the server fetches a URL the user chose, so:
- Only `http` / `https` on ports 80 and 443. No `user:pass@` URLs, no `localhost` / `*.internal`.
- Every connection, including after a redirect (max 5), must resolve to a public IP. This is checked on the resolved address at dial time (`checkLookupDial`), so DNS names pointing at private ranges and DNS rebinding are blocked. Refused ranges include loopback, RFC 1918, link-local (cloud metadata `169.254.169.254`), CGNAT, multicast, Fly's private `fdaa::/16` (ULA) and NAT64.
- Limits: 10 s timeout, only the first 2 MB of the page, 64 KB of headers, `text/html` only, no proxy, at most 4 lookups at once (429 beyond).
- No cookies or auth headers are sent. The User-Agent identifies the bot: `GopherDebtBot/1.0`.

## 🔧 ai_guard.go (AI guardrails)

These apply to every OpenAI call (receipt scans, wish page reading, name shortening):

| Guardrail | Details |
|-----------|---------|
| **Daily limit per user** | `db.TakeAIQuota` counts calls per user, day (UTC) and kind in `ai_usage`, in one atomic statement, so parallel requests can't overshoot. Limits: `AI_DAILY_RECEIPT_SCANS` (default 20) and `AI_DAILY_WISH_LOOKUPS` (default 50, text is cheaper than receipt images); `0` turns that AI use off. A receipt over the limit gets 429, so the frontend reads it on-device (OCR). A wish lookup gets status `limit`, or `ai_limit: true` with the shop data it found. A quota database error fails closed (no AI call). |
| **Token usage log** | Each call logs `AI usage: <kind> user <id> model <model>: N input + M output tokens`, from the API's `usage` block. |
| **Lookup cache** | Complete wish lookups (name **and** price, no limit hit) are kept in memory for 24 h (max 500 links), so the same link costs nothing again. Partial or failed answers aren't cached. |
| **No AI for pages without prices** | `hasPriceText`: if the page text has nothing like "€ 12,50" / "$9.99" / "25 EUR", it's not a product page, so the AI isn't asked. |
| **Free first** (`wish_lookup.go`) | Shop structured data is used whenever present; bot pages never go to the AI; long names are only shortened when over 64 characters. |

## 🔧 wish_lists.go

Shared GoWish groups, separate from debt groups. Every member has full access: rename, add/remove members, add/edit/delete any wish.

### Endpoints

| Method | Endpoint | Function | Description |
|--------|----------|----------|-------------|
| GET | `/api/wish-lists` | `GetWishLists` | Groups the caller belongs to |
| POST | `/api/wish-lists` | `CreateWishList` | Create a group with name, description, icon and initial members; caller is always a member |
| GET | `/api/wish-lists/:id` | `GetWishList` | Group with members |
| PUT | `/api/wish-lists/:id` | `UpdateWishList` | Edit name, description and icon |
| POST | `/api/wish-lists/:id/members` | `AddMember` | Add a user (`409` if already a member) |
| DELETE | `/api/wish-lists/:id/members/:userId` | `RemoveMember` | Remove a member or leave (their wishes in the group are deleted); returns `list_deleted` |
| GET | `/api/wish-lists/:id/items` | `GetItems` | Wishes, each with `added_by` |
| POST | `/api/wish-lists/:id/items` | `CreateItem` | Add a wish (validated like `/api/wish`) |
| PUT | `/api/wish-lists/:id/items/:itemId` | `UpdateItem` | Edit any wish in the group |
| DELETE | `/api/wish-lists/:id/items/:itemId` | `DeleteItem` | Delete any wish in the group |

### Access rules

- Every route checks membership first (`memberListID`). Non-members get `404`, so group IDs can't be probed.
- Item queries filter on both item ID and `list_id`, so an item from another group can't be edited by guessing its ID.
- Each wish belongs to the member who added it (`user_id`), which is their section in the group. Edits by other members don't change the owner.
- When a member leaves or is removed, their wishes in that group are deleted. When the last member leaves, the group is deleted too (in one transaction with a row lock, so two members leaving at once can't leave an orphan group).

---

## 🔧 access_control.go

### Handler Struct

```go
type AccessControlHandler struct {
    DB *sql.DB
}
```

### Endpoints (Founder Only)

All access control endpoints verify the requester is the founder before proceeding.

| Method | Endpoint | Function | Description |
|--------|----------|----------|-------------|
| GET | `/api/whitelist` | `GetWhitelist` | List all whitelisted emails |
| POST | `/api/whitelist` | `AddToWhitelist` | Add email to whitelist |
| DELETE | `/api/whitelist/:id` | `RemoveFromWhitelist` | Remove from whitelist |
| GET | `/api/blacklist` | `GetBlacklist` | List all blacklisted emails |
| POST | `/api/blacklist` | `AddToBlacklist` | Add email to blacklist (cannot blacklist founder) |
| DELETE | `/api/blacklist/:id` | `RemoveFromBlacklist` | Remove from blacklist |
| POST | `/api/admin/reset-finances` | `ResetFinances` | Truncate all expenses, payments and settlements (keeps users and groups) |

---

## 🔧 receipt.go

### Handler Struct

```go
type ReceiptHandler struct{}  // No DB needed — uses external API only
```

### Endpoints

| Method | Endpoint | Function | Description |
|--------|----------|----------|-------------|
| POST | `/api/receipt/scan` | `ScanReceipt` | Upload receipt image → AI extracts items & total |
| GET | `/api/receipt/config` | `ReceiptConfig` | Whether `OPENAI_API_KEY` is set |

### How It Works

```
1. Reject bodies over 14 MB (`MaxReceiptBytes`) with 413: up front via Content-Length,
   and while reading via http.MaxBytesReader. The cap keeps memory bounded on the 256 MB VM
   (OpenAI itself accepts far more); the frontend shrinks photos over 4 MB before uploading
2. Take one of 2 scan slots (`receiptScanSlots`), else 429 "busy"
3. Read the "image" file (above 8 MB it spills to a temp file, see MaxMultipartMemory in main.go)
4. Sniff the real type (http.DetectContentType). Only JPEG, PNG, WEBP and GIF are accepted (what
   OpenAI reads); anything else, including HEIC, gets 415. The frontend converts HEIC to JPEG first
5. Stream the image as a base64 data URL straight into the JSON body (one encoded copy in memory)
6. POST to the OpenAI Responses API (`/v1/responses`). The model is `OPENAI_RECEIPT_MODEL` or
   `gpt-6-luna`, image detail is "high", and the key goes in the `Authorization` header, never the
   URL. 60 s timeout, cancelled if the client goes away
7. Structured Outputs (`text.format` json_schema, strict) forces the answer into
   `{store_name, items[{name, price}], total}`
8. Return that JSON text to the frontend as `{"text": ...}`

OpenAI errors: 429 (rate/quota) → 429, so the frontend falls back to on-device OCR. Other errors → 502.
A model refusal → 422. An empty answer → 500.
```

**Memory:** a max-size (14 MB) scan peaks at about 46 MB of heap, so 2 concurrent scans fit comfortably in the 256 MB VM. (Before this change, a 40 MB upload peaked at about 253 MB.)

- Receipts in any language are supported (the model translates item names to English)
- Requires the `OPENAI_API_KEY` environment variable; `OPENAI_RECEIPT_MODEL` optionally overrides the model
