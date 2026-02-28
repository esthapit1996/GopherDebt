package models

import "time"

// User represents a user in the system
type User struct {
	ID           int       `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Name         string    `json:"name"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CreateUserRequest is the payload for creating a new user
type CreateUserRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
	Name     string `json:"name" binding:"required"`
}

// LoginRequest is the payload for user login
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse is returned after successful login
type LoginResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

// Group represents an expense sharing group
type Group struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedBy   int       `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Members     []User    `json:"members,omitempty"`
}

// CreateGroupRequest is the payload for creating a new group
type CreateGroupRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

// AddMemberRequest is the payload for adding a member to a group
type AddMemberRequest struct {
	UserID int `json:"user_id" binding:"required"`
}

// Expense represents an expense in a group
type Expense struct {
	ID          int            `json:"id"`
	GroupID     int            `json:"group_id"`
	PaidBy      int            `json:"paid_by"`
	PaidByUser  *User          `json:"paid_by_user,omitempty"`
	Amount      float64        `json:"amount"`
	Description string         `json:"description"`
	SplitType   string         `json:"split_type"`
	Splits      []ExpenseSplit `json:"splits,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// ExpenseSplit represents how an expense is split among users
type ExpenseSplit struct {
	ID        int     `json:"id"`
	ExpenseID int     `json:"expense_id"`
	UserID    int     `json:"user_id"`
	User      *User   `json:"user,omitempty"`
	Amount    float64 `json:"amount"`
}

// CreateExpenseRequest is the payload for creating a new expense
type CreateExpenseRequest struct {
	Amount      float64             `json:"amount" binding:"required,gt=0"`
	Description string              `json:"description" binding:"required"`
	SplitType   string              `json:"split_type" binding:"required,oneof=equal exact percentage"`
	SplitWith   []ExpenseSplitInput `json:"split_with"`
}

// ExpenseSplitInput is used when creating an expense with exact/percentage split
type ExpenseSplitInput struct {
	UserID int     `json:"user_id" binding:"required"`
	Amount float64 `json:"amount"`
}

// Settlement represents a payment between two users
type Settlement struct {
	ID        int       `json:"id"`
	GroupID   int       `json:"group_id"`
	PaidBy    int       `json:"paid_by"`
	PaidTo    int       `json:"paid_to"`
	Amount    float64   `json:"amount"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateSettlementRequest is the payload for creating a settlement
type CreateSettlementRequest struct {
	PaidTo int     `json:"paid_to" binding:"required"`
	Amount float64 `json:"amount" binding:"required,gt=0"`
}

// Balance represents what one user owes another
type Balance struct {
	FromUser *User   `json:"from_user"`
	ToUser   *User   `json:"to_user"`
	Amount   float64 `json:"amount"`
}

// GroupBalance represents all balances in a group
type GroupBalance struct {
	GroupID  int       `json:"group_id"`
	Balances []Balance `json:"balances"`
}

// UserBalance represents a user's total balance
type UserBalance struct {
	User    *User   `json:"user"`
	Balance float64 `json:"balance"`
}

// APIResponse is a standard API response wrapper
type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}
