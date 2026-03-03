# 📦 Models (`models/`)

Models define the **data structures** used throughout the application. They represent database entities, API requests, and API responses.

---

## 📚 Libraries Used

| Library | Import Path | Purpose |
|---------|-------------|---------|
| `time` | `time` | Timestamp fields (CreatedAt, UpdatedAt) |

---

## 📁 Files

| File | Purpose |
|------|---------|
| `models.go` | All data structures for the application |

---

## 🏗️ Entity Models

These represent records stored in the database.

### User

| Field | Type | JSON Tag | Description |
|-------|------|----------|-------------|
| `ID` | `int` | `id` | Primary key |
| `Email` | `string` | `email` | Unique email address |
| `PasswordHash` | `string` | `-` | Bcrypt hash (never sent to client!) |
| `Name` | `string` | `name` | Display name |
| `ThemePreference` | `string` | `theme_preference` | UI theme (dark, light, etc.) |
| `CreatedAt` | `time.Time` | `created_at` | When account was created |
| `UpdatedAt` | `time.Time` | `updated_at` | Last profile update |

### Group

| Field | Type | JSON Tag | Description |
|-------|------|----------|-------------|
| `ID` | `int` | `id` | Primary key |
| `Name` | `string` | `name` | Group name |
| `Description` | `string` | `description` | Optional description |
| `Emoji` | `string` | `emoji` | Group icon emoji |
| `CreatedBy` | `int` | `created_by` | User ID of creator |
| `Members` | `[]User` | `members,omitempty` | List of group members |
| `CreatedAt` | `time.Time` | `created_at` | When group was created |
| `UpdatedAt` | `time.Time` | `updated_at` | Last group update |

### GroupWithBalance

| Field | Type | JSON Tag | Description |
|-------|------|----------|-------------|
| `Group` | embedded | - | All Group fields |
| `MyBalance` | `float64` | `my_balance` | Current user's balance in this group |

### Expense

| Field | Type | JSON Tag | Description |
|-------|------|----------|-------------|
| `ID` | `int` | `id` | Primary key |
| `GroupID` | `int` | `group_id` | Which group this expense belongs to |
| `PaidBy` | `int` | `paid_by` | User ID of who paid |
| `PaidByUser` | `*User` | `paid_by_user,omitempty` | User object of payer |
| `Amount` | `float64` | `amount` | Total expense amount |
| `Description` | `string` | `description` | What the expense was for |
| `SplitType` | `string` | `split_type` | How to split: "equal", "exact", "percentage" |
| `Splits` | `[]ExpenseSplit` | `splits,omitempty` | How expense is divided |
| `CreatedAt` | `time.Time` | `created_at` | When expense was added |
| `UpdatedAt` | `time.Time` | `updated_at` | Last update |

### ExpenseSplit

| Field | Type | JSON Tag | Description |
|-------|------|----------|-------------|
| `ID` | `int` | `id` | Primary key |
| `ExpenseID` | `int` | `expense_id` | Which expense this split belongs to |
| `UserID` | `int` | `user_id` | Who owes this portion |
| `User` | `*User` | `user,omitempty` | User object |
| `Amount` | `float64` | `amount` | How much they owe |

### Settlement

| Field | Type | JSON Tag | Description |
|-------|------|----------|-------------|
| `ID` | `int` | `id` | Primary key |
| `GroupID` | `int` | `group_id` | Which group |
| `PaidBy` | `int` | `paid_by` | User ID of payer |
| `PaidTo` | `int` | `paid_to` | User ID of receiver |
| `Amount` | `float64` | `amount` | Payment amount |
| `CreatedAt` | `time.Time` | `created_at` | When payment was made |

### ExpensePayment

| Field | Type | JSON Tag | Description |
|-------|------|----------|-------------|
| `ID` | `int` | `id` | Primary key |
| `ExpenseID` | `int` | `expense_id` | Which expense being paid |
| `PaidBy` | `int` | `paid_by` | User ID of payer |
| `PaidByUser` | `*User` | `paid_by_user,omitempty` | User object |
| `Amount` | `float64` | `amount` | Payment amount |
| `Note` | `string` | `note,omitempty` | Optional payment note |
| `CreatedAt` | `time.Time` | `created_at` | When payment was made |

### Balance

| Field | Type | JSON Tag | Description |
|-------|------|----------|-------------|
| `FromUser` | `*User` | `from_user` | Who owes |
| `ToUser` | `*User` | `to_user` | Who is owed |
| `Amount` | `float64` | `amount` | How much is owed |

### Activity

| Field | Type | JSON Tag | Description |
|-------|------|----------|-------------|
| `ID` | `int` | `id` | Primary key |
| `GroupID` | `int` | `group_id` | Which group |
| `UserID` | `int` | `user_id` | Who performed action |
| `User` | `*User` | `user,omitempty` | User object |
| `ActionType` | `string` | `action_type` | Type of action (see activity.go) |
| `Description` | `string` | `description` | Human-readable description |
| `Amount` | `*float64` | `amount,omitempty` | Amount involved (if applicable) |
| `RelatedUserID` | `*int` | `related_user_id,omitempty` | Another user involved |
| `RelatedUser` | `*User` | `related_user,omitempty` | Related user object |
| `CreatedAt` | `time.Time` | `created_at` | When action occurred |

---

## 📝 Request Models

These define the expected JSON payload for API requests.

### CreateUserRequest

| Field | Type | Validation | Description |
|-------|------|------------|-------------|
| `Email` | `string` | `required,email` | Must be valid email |
| `Password` | `string` | `required,min=6` | At least 6 characters |
| `Name` | `string` | `required` | Display name |

### LoginRequest

| Field | Type | Validation | Description |
|-------|------|------------|-------------|
| `Email` | `string` | `required,email` | User's email |
| `Password` | `string` | `required` | User's password |

### CreateGroupRequest

| Field | Type | Validation | Description |
|-------|------|------------|-------------|
| `Name` | `string` | `required` | Group name |
| `Description` | `string` | `max=128` | Optional description |
| `Emoji` | `string` | - | Group icon (defaults to 💰) |

### AddMemberRequest

| Field | Type | Validation | Description |
|-------|------|------------|-------------|
| `UserID` | `int` | `required` | User to add to group |

### CreateExpenseRequest

| Field | Type | Validation | Description |
|-------|------|------------|-------------|
| `Amount` | `float64` | `required,gt=0` | Must be positive |
| `Description` | `string` | `required` | What expense is for |
| `SplitType` | `string` | `required,oneof=equal exact percentage` | How to split |
| `SplitWith` | `[]ExpenseSplitInput` | - | Custom splits (for exact/percentage) |

### ExpenseSplitInput

| Field | Type | Validation | Description |
|-------|------|------------|-------------|
| `UserID` | `int` | `required` | Who owes |
| `Amount` | `float64` | - | Their share (amount or percentage) |

### CreateSettlementRequest

| Field | Type | Validation | Description |
|-------|------|------------|-------------|
| `PaidTo` | `int` | `required` | User receiving payment |
| `Amount` | `float64` | `required,gt=0` | Payment amount |

### CreateExpensePaymentRequest

| Field | Type | Validation | Description |
|-------|------|------------|-------------|
| `Amount` | `float64` | `required,gt=0` | Payment amount |
| `Note` | `string` | - | Optional note |

### CreateStashExpenseRequest

| Field | Type | Validation | Description |
|-------|------|------------|-------------|
| `Amount` | `float64` | `required,gt=0` | Must be positive |
| `Description` | `string` | `max=255` | Optional (defaults to category name or "Expense") |
| `Category` | `string` | `max=50` | Optional category tag |

---

## 📤 Response Models

### LoginResponse

| Field | Type | JSON Tag | Description |
|-------|------|----------|-------------|
| `Token` | `string` | `token` | JWT token for future requests |
| `User` | `User` | `user` | Logged in user's profile |

### APIResponse

| Field | Type | JSON Tag | Description |
|-------|------|----------|-------------|
| `Success` | `bool` | `success` | Whether request succeeded |
| `Message` | `string` | `message,omitempty` | Success message |
| `Data` | `interface{}` | `data,omitempty` | Response payload |
| `Error` | `string` | `error,omitempty` | Error message |

### ExpensePaymentStatus

| Field | Type | JSON Tag | Description |
|-------|------|----------|-------------|
| `TotalOwed` | `float64` | `total_owed` | Total amount owed on expense |
| `TotalPaid` | `float64` | `total_paid` | Total amount paid so far |

### StashExpense (GopherStash)

| Field | Type | JSON Tag | Description |
|-------|------|----------|-------------|
| `ID` | `int` | `id` | Primary key |
| `UserID` | `int` | `user_id` | Owner of this expense |
| `Amount` | `float64` | `amount` | Expense amount |
| `Description` | `string` | `description` | What the expense was for |
| `Category` | `string` | `category` | Category (food, transport, etc.) |
| `CreatedAt` | `time.Time` | `created_at` | When expense was added |

### StashSummary

| Field | Type | JSON Tag | Description |
|-------|------|----------|-------------|
| `TotalSpent` | `float64` | `total_spent` | Sum of all expenses |
| `ExpenseCount` | `int` | `expense_count` | Number of expenses |
| `ByCategory` | `map[string]float64` | `by_category` | Total per category |

---

## 🏷️ JSON Tag Meanings

| Tag | Meaning |
|-----|---------|
| `json:"field_name"` | Maps Go field to JSON key |
| `json:"-"` | Never include in JSON (like passwords) |
| `json:"field,omitempty"` | Omit if value is zero/nil/empty |
| `binding:"required"` | Gin validation: field must be present |
| `binding:"email"` | Gin validation: must be valid email format |
| `binding:"min=6"` | Gin validation: minimum string length |
| `binding:"gt=0"` | Gin validation: greater than 0 |
| `binding:"oneof=a b c"` | Gin validation: must be one of listed values |
