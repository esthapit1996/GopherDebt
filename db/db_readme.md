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
| `migrations.go` | Database schema creation and updates |

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
| `CreateUser` | `db *sql.DB, email, passwordHash, name string` | `*models.User, error` | Creates a new user with default theme 'dark' |
| `GetUserByEmail` | `db *sql.DB, email string` | `*models.User, error` | Finds user by email (used for login) |
| `GetUserByID` | `db *sql.DB, id int` | `*models.User, error` | Finds user by their ID |
| `GetAllUsers` | `db *sql.DB` | `[]models.User, error` | Returns all users (for member selection) |
| `UpdateUserTheme` | `db *sql.DB, userID int, theme string` | `error` | Updates user's theme preference |

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

---

## 🔧 settlements.go

### Functions

| Function | Parameters | Returns | Description |
|----------|------------|---------|-------------|
| `CreateSettlement` | `db, groupID, paidBy, paidTo int, amount float64` | `*models.Settlement, error` | Records a direct payment between users |
| `GetGroupSettlements` | `db *sql.DB, groupID int` | `[]models.Settlement, error` | Lists all settlements in a group |
| `CalculateGroupBalances` | `db *sql.DB, groupID int` | `[]models.Balance, error` | **The brain!** Calculates who owes whom |
| `CalculateUserBalance` | `db *sql.DB, groupID, userID int` | `float64, error` | Gets single user's balance in a group |
| `GetDebtOverview` | `db *sql.DB, userID int` | `[]models.UserBalance, error` | Shows user's debts across all groups |
| `GetUserPaymentHistory` | `db *sql.DB, userID int, limit int` | `[]PaymentHistoryItem, error` | Gets all payments made/received by user |

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
| `RunMigrations` | `db *sql.DB` | `error` | Creates all tables and indexes if they don't exist |

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
