# GopherDebt - Splitwise-like Backend

A Go backend for expense sharing built with Gin framework and PostgreSQL.

## Setup

```bash
# Install dependencies
go mod tidy

# Build
go build -o gopherdebt .

# Run (set DATABASE_URL env var for production)
./gopherdebt
```

## Environment Variables

- `DATABASE_URL` - PostgreSQL connection string
- `JWT_SECRET` - Secret key for JWT tokens (default: "your-secret-key-change-in-production")
- `PORT` - Server port (default: 8080)

## API Endpoints

### Authentication

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/register` | Register a new user |
| POST | `/api/login` | Login and get JWT token |

### Users (Protected)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/profile` | Get current user profile |
| GET | `/api/users` | Get all users |

### Groups (Protected)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/groups` | Create a new group |
| GET | `/api/groups` | Get user's groups |
| GET | `/api/groups/:id` | Get group details |
| DELETE | `/api/groups/:id` | Delete a group (creator only) |
| POST | `/api/groups/:id/members` | Add a member to group |
| DELETE | `/api/groups/:id/members/:memberID` | Remove a member |

### Expenses (Protected)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/groups/:id/expenses` | Create expense |
| GET | `/api/groups/:id/expenses` | Get group expenses |
| GET | `/api/groups/:id/expenses/:expenseID` | Get expense details |
| DELETE | `/api/groups/:id/expenses/:expenseID` | Delete expense (payer only) |

### Settlements & Balances (Protected)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/groups/:id/settlements` | Record a settlement |
| GET | `/api/groups/:id/settlements` | Get group settlements |
| GET | `/api/groups/:id/balances` | Get who owes whom |
| GET | `/api/groups/:id/my-balance` | Get your balance |

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

## Project Structure

```
├── main.go           # Entry point
├── db/
│   ├── migrations.go # Database schema
│   ├── users.go      # User operations
│   ├── groups.go     # Group operations
│   ├── expenses.go   # Expense operations
│   └── settlements.go# Settlement & balance operations
├── handlers/
│   ├── users.go      # User handlers
│   ├── groups.go     # Group handlers
│   ├── expenses.go   # Expense handlers
│   └── settlements.go# Settlement handlers
├── middleware/
│   └── auth.go       # JWT authentication
└── models/
    └── models.go     # Data models
```
