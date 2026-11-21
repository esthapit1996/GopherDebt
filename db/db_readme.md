# 📦 Database Layer (`db/`)

This folder contains all database operations for GopherDebt. Each file handles CRUD operations for a specific domain.

---

## 📚 Libraries Used

| Library | Import Path | Purpose |
|---------|-------------|---------|
| `database/sql` | `database/sql` | Go's standard SQL database interface |
| `models` | `gopherdebt/models` | Our data structures (User, Group, Expense, etc.) |

---

## 📁 Files Overview

| File | Purpose |
|------|---------|
| `users.go` | User authentication and profile management |
| `groups.go` | Group creation, membership, and retrieval |
| `expenses.go` | Expense creation, splits, and queries |
| `settlements.go` | Direct payments between users + balance calculation |
| `expense_payments.go` | Partial payments toward specific expenses |
| `activity.go` | Activity logging for group history |
| `suggestions.go` | Suggestion box with voting and comments |
| `stash.go` | GopherStash personal expense tracking |
| `wish.go` | GoWish personal wishlist |
| `wish_lists.go` | GoWish shared wish groups |
| `gocrumbs.go` | GoCrumbs budget profile, expense lines and extra pay lines |
| `access_control.go` | Email whitelist/blacklist management |
| `retry.go` | Generic retry with backoff for transient DB errors |
| `migrations.go` | Database schema creation and updates |
| `security.go` | Locks Supabase's REST API out of the app tables; case-insensitive unique emails |
| `ai_usage.go` | Per-user daily AI call counts (`TakeAIQuota`) |

---

## 🔧 users.go

### Variables

| Variable | Type | Purpose |
|----------|------|---------|
| `ErrNotFound` | `error` | Returned when a database record doesn't exist |
| `ErrDuplicate` | `error` | Returned when trying to create a duplicate record |

### Functions

| Function | Parameters | Returns | Description |
|----------|------------|---------|-------------|
| `CreateUser` | `db *sql.DB, email, passwordHash, name string` | `*models.User, error` | Creates a new user with defaults (theme `darkknight`, language `en`) |
| `GetUserByEmail` | `db *sql.DB, email string` | `*models.User, error` | Finds user by email, ignoring letter case (oldest account wins if case variants exist); used for login and the register duplicate check |
| `GetUserByID` | `db *sql.DB, id int` | `*models.User, error` | Finds user by their ID |
| `GetAllUsers` | `db *sql.DB` | `[]models.User, error` | Returns all users except deleted ones (for member selection) |
| `UpdateUserTheme` | `db *sql.DB, userID int, theme string` | `error` | Updates user's theme preference |
| `UpdateUserPassword` | `db *sql.DB, userID int, newPasswordHash string` | `int, error` | Sets the password and bumps `token_version` (logs out other sessions); returns the new version |
| `GetSessionState` | `db *sql.DB, userID int` | `tokenVersion int, active bool, err error` | Used by `middleware.ActiveUser` on every request. `active` is false if the account is deleted or its email is blacklisted |
| `DeleteUser` | `db *sql.DB, userID int` | `error` | Anonymizes the account (see `handlers_readme.md`, "Deleting a user"); `ErrNotFound` if already deleted |

---

## 🔧 groups.go

### Functions

| Function | Parameters | Returns | Description |
|----------|------------|---------|-------------|
| `CreateGroup` | `db, name, description, emoji string, createdBy int` | `*models.Group, error` | Creates group and adds creator as first member (uses transaction) |
| `GetGroupByID` | `db *sql.DB, groupID int` | `*models.Group, error` | Gets group details with all members |
| `GetUserGroups` | `db *sql.DB, userID int` | `[]models.Group, error` | Gets all groups a user belongs to |
| `AddGroupMember` | `db *sql.DB, groupID, userID int` | `error` | Adds a user to a group |
| `RemoveGroupMember` | `db *sql.DB, groupID, userID int` | `error` | Removes a user from a group |
| `GetGroupMembers` | `db *sql.DB, groupID int` | `[]models.User, error` | Lists all members of a group |
| `IsGroupMember` | `db *sql.DB, groupID, userID int` | `bool, error` | Checks if user is in group |

---

## 🔧 expenses.go

### Functions

| Function | Parameters | Returns | Description |
|----------|------------|---------|-------------|
| `CreateExpense` | `db, groupID, paidBy int, amount float64, description, splitType string, splits []ExpenseSplitInput` | `*models.Expense, error` | Creates expense with splits (transaction) |
| `GetExpenseByID` | `db *sql.DB, expenseID int` | `*models.Expense, error` | Gets expense with payer info and splits |
| `GetGroupExpenses` | `db *sql.DB, groupID int` | `[]models.Expense, error` | Lists all expenses in a group |
| `GetExpenseSplits` | `db *sql.DB, expenseID int` | `[]models.ExpenseSplit, error` | Gets how an expense is split |
| `DeleteExpense` | `db *sql.DB, expenseID int` | `error` | Deletes an expense (cascades to splits) |
| `ClearGroupFinances` | `db *sql.DB, groupID int` | `expenses, settlements int64, err error` | Deletes all of a group's expenses (splits and expense payments cascade) **and** settlements in one transaction, so every balance ends at 0. Settlements must go too, or they'd show up as new debts |

---

## 🔧 settlements.go

### Functions

| Function | Parameters | Returns | Description |
|----------|------------|---------|-------------|
| `CreateSettlement` | `db, groupID, paidBy, paidTo int, amount float64` | `*models.Settlement, error` | Records a direct payment between users |
| `GetGroupSettlements` | `db *sql.DB, groupID int` | `[]models.Settlement, error` | Lists all settlements in a group |
| `CalculateGroupBalances` | `db *sql.DB, groupID int` | `[]models.Balance, error` | **The brain!** Calculates who owes whom |
| `GetUserBalanceInGroup` | `db *sql.DB, groupID, userID int` | `float64, error` | Gets single user's balance in a group |
| `GetDebtOverview` | `db *sql.DB, userID int` | `[]DebtOverviewItem, error` | Shows user's net debts across all groups |
| `GetDebtDetails` | `db *sql.DB, userID, otherUserID int` | `[]DebtDetailItem, error` | Breakdown of debts between two users |
| `GetPaymentHistory` | `db *sql.DB, userID int` | `[]PaymentHistoryItem, error` | Payments made/received by the user, after their `history_cleared_at` |
| `ClearPaymentHistory` | `db *sql.DB, userID int` | `error` | Sets `users.history_cleared_at` to now. Hides history for that user only; never deletes settlements or payments |

### Balance Calculation Logic
```
For each expense:
  - payer gets +split_amount (they're owed)
  - each split member gets -split_amount (they owe)

For each settlement:
  - payer gets +amount
  - receiver gets -amount

For each expense_payment:
  - payer gets +amount
  - expense owner gets -amount
```

---

## 🔧 expense_payments.go

### Functions

| Function | Parameters | Returns | Description |
|----------|------------|---------|-------------|
| `CreateExpensePayment` | `db, expenseID, paidBy int, amount float64, note string` | `*models.ExpensePayment, error` | Records partial payment on an expense |
| `GetExpensePayments` | `db *sql.DB, expenseID int` | `[]models.ExpensePayment, error` | Lists all payments for an expense |
| `GetTotalPaymentsForExpense` | `db, expenseID, userID int` | `float64, error` | How much a user has paid on an expense |
| `GetAllPaymentsForExpense` | `db *sql.DB, expenseID int` | `float64, error` | Total paid by everyone on an expense |
| `DeleteExpensePayment` | `db *sql.DB, paymentID int` | `error` | Deletes a payment record |
| `GetExpensePaymentByID` | `db *sql.DB, paymentID int` | `*models.ExpensePayment, error` | Gets a single payment by ID |

---

## 🔧 activity.go

### Constants

| Constant | Value | When Used |
|----------|-------|-----------|
| `ActivityExpenseCreated` | `"expense_created"` | New expense added |
| `ActivityExpenseDeleted` | `"expense_deleted"` | Expense removed |
| `ActivitySettlement` | `"settlement"` | Direct payment made |
| `ActivityPayment` | `"payment"` | Expense payment made |
| `ActivityMemberAdded` | `"member_added"` | User joined group |
| `ActivityMemberRemoved` | `"member_removed"` | User left group |
| `ActivityGroupCreated` | `"group_created"` | New group created |

### Functions

| Function | Parameters | Returns | Description |
|----------|------------|---------|-------------|
| `LogActivity` | `db, groupID, userID int, actionType, description string, amount *float64, relatedUserID *int` | `error` | Records an activity event |
| `GetGroupActivities` | `db *sql.DB, groupID, limit int` | `[]models.Activity, error` | Gets activity feed for a group |

---

## 🔧 migrations.go

### Functions

| Function | Parameters | Returns | Description |
|----------|------------|---------|-------------|
| `RunMigrations` | `db *sql.DB` | `error` | Creates all tables and indexes if they don't exist, then runs `lockDownPublicAPI` and `ensureUniqueEmailIgnoringCase` |

### Tables Created

| Table | Purpose |
|-------|---------|
| `users` | User accounts (email, password_hash, name, theme) |
| `groups` | Expense sharing groups (name, description, emoji) |
| `group_members` | Many-to-many: which users are in which groups |
| `expenses` | Individual expenses (amount, description, who paid) |
| `expense_splits` | How each expense is divided among users |
| `settlements` | Direct payments between users |
| `expense_payments` | Partial payments toward specific expenses |
| `activity_log` | History of all actions in a group |
| `suggestions` | User feature suggestions |
| `suggestion_votes` | Anonymous voting on suggestions |
| `suggestion_comments` | Comments on suggestions |
| `stash_expenses` | GopherStash personal expenses |
| `wish_items` | GoWish wishes (`list_id` NULL = personal, set = in a shared group) |
| `wish_lists` | GoWish shared wish groups |
| `wish_list_members` | Members of each shared wish group |
| `ai_usage` | AI calls per user, day and kind (`receipt`, `wish`), for the daily limits; rows older than 30 days are deleted at boot |
| `gocrumbs_profiles` | One GoCrumbs budget profile per user (inputs only: work type, gross, tax class, church tax, Bundesland, children, Zusatzbeitrag, rent, savings, and the Werkstudent's `ws_hourly_wage`, `ws_hours`, `ws_hours_period`). Employee and Werkstudent income have separate columns, so switching work type overwrites neither. The `ws_*` columns are added with `ADD COLUMN IF NOT EXISTS`. CHECK constraints mirror the API's ranges |
| `gocrumbs_expenses` | A profile's named monthly expense lines (`kind` = `subscription` or `custom`, with `position`) |
| `gocrumbs_extra_pay` | A profile's flat monthly extra pay lines, e.g. on-call (`name`, `amount`, `tax_free`, with `position`) |
| `email_whitelist` | Approved registration emails |
| `email_blacklist` | Blocked registration emails |

---

## 🔄 Common Patterns

### Transaction Pattern
```go
tx, err := db.Begin()
if err != nil {
    return nil, err
}
defer tx.Rollback()  // Rollback if anything fails

// ... do operations with tx ...

if err := tx.Commit(); err != nil {
    return nil, err
}
```

### Null Handling Pattern
```go
var description sql.NullString
// ... scan into description ...
if description.Valid {
    group.Description = description.String
}
```

### Error Check Pattern
```go
if err == sql.ErrNoRows {
    return nil, ErrNotFound  // Our custom error
}
if err != nil {
    return nil, err  // Other database errors
}
```

---

## 🔧 stash.go

### Functions

| Function | Parameters | Returns | Description |
|----------|------------|---------|-------------|
| `CreateStashExpense` | `db *sql.DB, userID int, amount float64, description, category string` | `*models.StashExpense, error` | Adds a personal expense to the user's stash |
| `GetStashExpenses` | `db *sql.DB, userID int` | `[]models.StashExpense, error` | Returns all personal expenses for a user (newest first) |
| `UpdateStashExpense` | `db *sql.DB, expenseID, userID int, amount float64, description, category string` | `*models.StashExpense, error` | Updates a personal expense (only if owned by user) |
| `DeleteStashExpense` | `db *sql.DB, expenseID, userID int` | `error` | Removes a personal expense (only if owned by user) |
| `GetStashSummary` | `db *sql.DB, userID int` | `*models.StashSummary, error` | Total spent + breakdown by category |
| `ClearStashExpenses` | `db *sql.DB, userID int` | `error` | Removes all personal expenses for a user |

---

## 🔧 wish.go

### Functions

| Function | Parameters | Returns | Description |
|----------|------------|---------|-------------|
| `CreateWishItem` | `db *sql.DB, userID int, name, why, link string, cost *float64` | `*models.WishItem, error` | Adds an item to the user's wishlist |
| `GetWishItems` | `db *sql.DB, userID int` | `[]models.WishItem, error` | Returns all wishlist items for a user (newest first) |
| `UpdateWishItem` | `db *sql.DB, itemID, userID int, name, why, link string, cost *float64` | `*models.WishItem, error` | Updates a wishlist item (only if owned by user) |
| `DeleteWishItem` | `db *sql.DB, itemID, userID int` | `error` | Removes a wishlist item (only if owned by user) |

All personal functions only touch wishes with `list_id IS NULL`.

---

## 🔧 wish_lists.go

### Functions

| Function | Parameters | Returns | Description |
|----------|------------|---------|-------------|
| `CreateWishList` | `db *sql.DB, userID int, name, description, emoji string, memberIDs []int` | `*models.WishList, error` | Creates a group with the creator as first member plus `memberIDs` (unknown IDs skipped) in one transaction; blank emoji defaults to 👥 |
| `GetUserWishLists` | `db *sql.DB, userID int` | `[]models.WishList, error` | Groups the user belongs to, newest first, with members, item count and total cost |
| `GetWishList` | `db *sql.DB, listID int` | `*models.WishList, error` | One group with members |
| `UpdateWishList` | `db *sql.DB, listID int, name, description, emoji string` | `error` | Edits a group's name, description and icon (blank emoji = 👥) |
| `IsWishListMember` | `db *sql.DB, listID, userID int` | `bool, error` | Membership check |
| `AddWishListMember` | `db *sql.DB, listID, userID int` | `error` | Adds a member; `ErrAlreadyMember` if already in |
| `RemoveWishListMember` | `db *sql.DB, listID, userID int` | `bool, error` | Removes a member and their wishes in the group; deletes the group when the last member leaves and returns `true` |
| `GetWishListItems` | `db *sql.DB, listID int` | `[]models.WishItem, error` | Wishes in a group with `AddedBy`, newest first |
| `CreateWishListItem` | `db *sql.DB, listID, userID int, name, why, link string, cost *float64` | `*models.WishItem, error` | Adds a wish to a group |
| `UpdateWishListItem` | `db *sql.DB, listID, itemID int, name, why, link string, cost *float64` | `*models.WishItem, error` | Edits a wish in a group |
| `DeleteWishListItem` | `db *sql.DB, listID, itemID int` | `error` | Deletes a wish from a group |

---

## 🔧 gocrumbs.go

### Functions

| Function | Parameters | Returns | Description |
|----------|------------|---------|-------------|
| `GetGoCrumbsProfile` | `db *sql.DB, userID int` | `*models.GoCrumbsProfile, error` | The user's saved profile with its expense and extra pay lines (empty lists, never nil); `ErrNotFound` before the first save |
| `SaveGoCrumbsProfile` | `db *sql.DB, userID int, req *models.GoCrumbsRequest` | `*models.GoCrumbsProfile, error` | Creates or replaces the profile, its expense lines and its extra pay lines in one transaction and returns the saved profile |

`SaveGoCrumbsProfile` upserts the profile row first, which holds the row lock until commit, so two parallel saves by one user run one after the other and can't duplicate lines. `MaxGoCrumbsLines` (50) caps subscriptions plus custom lines and `MaxGoCrumbsExtraPayLines` (20) caps extra pay lines; the handler enforces both.

---

## 🔧 access_control.go

### Types

| Type | Purpose |
|------|---------|
| `WhitelistEntry` | Represents an approved email with metadata |
| `BlacklistEntry` | Represents a blocked email with reason |

### Functions

| Function | Parameters | Returns | Description |
|----------|------------|---------|-------------|
| `IsEmailWhitelisted` | `db *sql.DB, email string` | `bool, error` | Checks if email is in the whitelist |
| `IsEmailBlacklisted` | `db *sql.DB, email string` | `bool, error` | Checks if email is in the blacklist |
| `GetWhitelist` | `db *sql.DB` | `[]WhitelistEntry, error` | Returns all whitelisted emails |
| `GetBlacklist` | `db *sql.DB` | `[]BlacklistEntry, error` | Returns all blacklisted emails |
| `AddToWhitelist` | `db *sql.DB, email string, addedBy int` | `*WhitelistEntry, error` | Adds email to whitelist |
| `AddToBlacklist` | `db *sql.DB, email, reason string, addedBy int` | `*BlacklistEntry, error` | Adds email to blacklist |
| `RemoveFromWhitelist` | `db *sql.DB, id int` | `error` | Removes entry from whitelist |
| `RemoveFromBlacklist` | `db *sql.DB, id int` | `error` | Removes entry from blacklist |

---

## 🔧 security.go

Runs at the end of `RunMigrations` on every boot.

| Function | Description |
|----------|-------------|
| `lockDownPublicAPI` | Closes Supabase's auto-generated REST API (PostgREST, roles `anon` / `authenticated`) over the app tables: drops the old always-true RLS policies and revokes those roles' table and sequence grants. RLS stays enabled with **no policies**, so those roles see nothing even if a grant comes back. |
| `ensureUniqueEmailIgnoringCase` | Creates a unique index on `LOWER(email)` so case variants of an email can't both register. If such duplicates already exist, it logs a warning and skips the index instead of failing boot. |

**Why the app still has access:** the app's role owns the tables, and owners bypass RLS unless it is forced. (Every boot already runs `ALTER TABLE ... ENABLE ROW LEVEL SECURITY`, which only the owner can do.) `lockDownPublicAPI` checks this first: if the role doesn't own every app table, a table has `FORCE ROW LEVEL SECURITY`, and the role has no `BYPASSRLS`, it changes nothing and logs `WARNING: public API lockdown skipped ...`. Only existing policies and grants are touched, so after the first run it does no DDL.

**Do not add `USING (true)` policies.** A policy without a `TO` clause applies to every role, including `anon`, and would reopen the REST API.

---

## 🔧 retry.go

### Constants

| Constant | Value | Purpose |
|----------|-------|---------|
| `maxRetries` | `3` | Number of retry attempts for transient errors |

### Functions

| Function | Parameters | Returns | Description |
|----------|------------|---------|-------------|
| `retry[T]` | `label string, fn func() (T, error)` | `T, error` | Generic retry with backoff (150ms, 300ms). Skips retry for `ErrNotFound`/`ErrDuplicate` |

### How It Works
```
1. Execute fn()
2. If success → return result
3. If ErrNotFound or ErrDuplicate → return immediately (not transient)
4. If other error → log, wait 150ms × attempt, retry
5. After 3 failures → log and return last error
```
