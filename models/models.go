package models

import "time"

// User represents a user in the system
type User struct {
	ID              int       `json:"id"`
	Email           string    `json:"email"`
	PasswordHash    string    `json:"-"`
	Name            string    `json:"name"`
	Avatar          string    `json:"avatar"`
	ThemePreference string    `json:"theme_preference"`
	Language        string    `json:"language"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	// Goes into login tokens; bumped on password change / account deletion to invalidate older tokens
	TokenVersion int `json:"-"`
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

// ChangePasswordRequest is the payload for changing a user's password
type ChangePasswordRequest struct {
	OldPassword     string `json:"old_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,min=6"`
	ConfirmPassword string `json:"confirm_password" binding:"required"`
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
	Emoji       string    `json:"emoji"`
	CreatedBy   int       `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Members     []User    `json:"members,omitempty"`
}

// GroupWithBalance includes the user's balance in the group
type GroupWithBalance struct {
	Group
	MyBalance float64 `json:"my_balance"`
}

// CreateGroupRequest is the payload for creating a new group
type CreateGroupRequest struct {
	Name        string `json:"name" binding:"required,max=69"`
	Description string `json:"description" binding:"max=128"`
	Emoji       string `json:"emoji"`
}

// UpdateGroupRequest is the payload for updating a group
type UpdateGroupRequest struct {
	Name        string `json:"name" binding:"required,max=69"`
	Description string `json:"description" binding:"max=128"`
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
	IsPaid      bool           `json:"is_paid"`
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
	Description string              `json:"description" binding:"required,max=420"`
	SplitType   string              `json:"split_type" binding:"required,oneof=equal exact percentage"`
	SplitWith   []ExpenseSplitInput `json:"split_with"`
	PaidBy      int                 `json:"paid_by"`
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

// ExpensePayment represents a partial payment towards an expense
type ExpensePayment struct {
	ID         int       `json:"id"`
	ExpenseID  int       `json:"expense_id"`
	PaidBy     int       `json:"paid_by"`
	PaidByUser *User     `json:"paid_by_user,omitempty"`
	Amount     float64   `json:"amount"`
	Note       string    `json:"note,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// CreateExpensePaymentRequest is the payload for recording a payment on an expense
type CreateExpensePaymentRequest struct {
	Amount float64 `json:"amount" binding:"required,gt=0"`
	Note   string  `json:"note"`
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

// Activity represents a logged action in a group
type Activity struct {
	ID            int       `json:"id"`
	GroupID       int       `json:"group_id"`
	UserID        int       `json:"user_id"`
	User          *User     `json:"user,omitempty"`
	ActionType    string    `json:"action_type"`
	Description   string    `json:"description"`
	Amount        *float64  `json:"amount,omitempty"`
	RelatedUserID *int      `json:"related_user_id,omitempty"`
	RelatedUser   *User     `json:"related_user,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// ExpensePaymentStatus represents the payment status for an expense
type ExpensePaymentStatus struct {
	TotalOwed float64 `json:"total_owed"`
	TotalPaid float64 `json:"total_paid"`
}

// StashExpense represents a personal expense tracked in GopherStash
type StashExpense struct {
	ID          int       `json:"id"`
	UserID      int       `json:"user_id"`
	Amount      float64   `json:"amount"`
	Description string    `json:"description"`
	Category    string    `json:"category"`
	CreatedAt   time.Time `json:"created_at"`
}

// CreateStashExpenseRequest is the payload for adding a personal expense
type CreateStashExpenseRequest struct {
	Amount      float64 `json:"amount" binding:"required,gt=0"`
	Description string  `json:"description" binding:"max=255"`
	Category    string  `json:"category" binding:"max=50"`
}

// StashSummary is the total spent summary for GopherStash
type StashSummary struct {
	TotalSpent   float64            `json:"total_spent"`
	ExpenseCount int                `json:"expense_count"`
	ByCategory   map[string]float64 `json:"by_category"`
}

// WishItem represents something the user wants to buy, tracked in GoWish
type WishItem struct {
	ID        int       `json:"id"`
	UserID    int       `json:"user_id"`
	Name      string    `json:"name"`
	Why       string    `json:"why"`
	Link      string    `json:"link"`
	Cost      *float64  `json:"cost"`
	CreatedAt time.Time `json:"created_at"`
	// Set only for items in a shared wish list
	ListID  *int        `json:"list_id,omitempty"`
	AddedBy *WishMember `json:"added_by,omitempty"`
}

// WishMember is the public view of a user in a shared wish list (no email)
type WishMember struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

// WishList is a shared GoWish list; every member can manage its items and members
type WishList struct {
	ID          int          `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Emoji       string       `json:"emoji"`
	CreatedAt   time.Time    `json:"created_at"`
	Members     []WishMember `json:"members"`
	ItemCount   int          `json:"item_count"`
	TotalCost   float64      `json:"total_cost"`
}

// WishListRequest is the payload for updating a shared wish list (same options as a debt group)
type WishListRequest struct {
	Name        string `json:"name" binding:"required,max=64"`
	Description string `json:"description" binding:"max=128"`
	Emoji       string `json:"emoji" binding:"max=10"`
}

// CreateWishListRequest also takes the initial members (the creator is always added)
type CreateWishListRequest struct {
	WishListRequest
	MemberIDs []int `json:"member_ids" binding:"max=50,dive,gt=0"`
}

// WishListMemberRequest is the payload for adding a member to a shared wish list
type WishListMemberRequest struct {
	UserID int `json:"user_id" binding:"required,gt=0"`
}

// WishItemRequest is the payload for adding or updating a GoWish item.
// Cost is optional: nil means no price set.
type WishItemRequest struct {
	Name string   `json:"name" binding:"required,max=64"`
	Why  string   `json:"why" binding:"max=128"`
	Link string   `json:"link" binding:"max=2048"`
	Cost *float64 `json:"cost" binding:"omitempty,gt=0"`
}

// GoCrumbsExpenseLine is one named monthly expense (a subscription or a custom line)
type GoCrumbsExpenseLine struct {
	Name     string  `json:"name" binding:"required,max=60"`
	Amount   float64 `json:"amount" binding:"gte=0,max=1000000"`
	Category string  `json:"category" binding:"max=30"`
}

// GoCrumbsExtraPayLine is a flat monthly payment on top of the salary, such as on-call pay.
// It is taxable unless TaxFree is set (e.g. Sunday and holiday surcharges).
type GoCrumbsExtraPayLine struct {
	Name    string  `json:"name" binding:"required,max=60"`
	Amount  float64 `json:"amount" binding:"gte=0,max=1000000"`
	TaxFree bool    `json:"tax_free"`
}

// GoCrumbsRequest is a GoCrumbs budget profile: the user's inputs, never computed results.
// Gross amounts are per GrossPeriod ("month" or "year"); everything else is EUR per month.
// Employee income (gross, side income, extra pay) and Werkstudent income (hourly wage and hours)
// are separate fields and both are stored; WorkType decides which one counts.
type GoCrumbsRequest struct {
	WorkType       string                 `json:"work_type" binding:"required,oneof=fte werkstudent"`
	GrossAmount    float64                `json:"gross_amount" binding:"gte=0,max=10000000"`
	GrossPeriod    string                 `json:"gross_period" binding:"required,oneof=month year"`
	SideGross      float64                `json:"side_gross" binding:"gte=0,max=100000"`
	WsHourlyWage   float64                `json:"ws_hourly_wage" binding:"gte=0,max=1000"`
	WsHours        float64                `json:"ws_hours" binding:"gte=0,max=744"`
	WsHoursPeriod  string                 `json:"ws_hours_period" binding:"omitempty,oneof=week month"`
	SteuerKlasse   int                    `json:"steuerklasse" binding:"required,min=1,max=6"`
	ChurchTax      bool                   `json:"church_tax"`
	Bundesland     string                 `json:"bundesland" binding:"required,oneof=BW BY BE BB HB HH HE MV NI NW RP SL SN ST SH TH"`
	Children       int                    `json:"children" binding:"min=0,max=20"`
	Under23        bool                   `json:"under_23"`
	Zusatzbeitrag  float64                `json:"zusatzbeitrag" binding:"gte=0,max=10"`
	Rent           float64                `json:"rent" binding:"gte=0,max=1000000"`
	Savings        float64                `json:"savings" binding:"gte=0,max=1000000"`
	Subscriptions  []GoCrumbsExpenseLine  `json:"subscriptions" binding:"max=50,dive"`
	CustomExpenses []GoCrumbsExpenseLine  `json:"custom_expenses" binding:"max=50,dive"`
	ExtraPay       []GoCrumbsExtraPayLine `json:"extra_pay" binding:"max=20,dive"`
}

// GoCrumbsProfile is a saved GoCrumbs profile
type GoCrumbsProfile struct {
	GoCrumbsRequest
	UpdatedAt time.Time `json:"updated_at"`
}

// APIResponse is a standard API response wrapper
type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}
