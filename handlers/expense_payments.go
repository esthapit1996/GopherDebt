package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"gopherdebt/db"
	"gopherdebt/models"
)

type ExpensePaymentHandler struct {
	DB *sql.DB
}

func NewExpensePaymentHandler(database *sql.DB) *ExpensePaymentHandler {
	return &ExpensePaymentHandler{DB: database}
}

// CreateExpensePayment records a payment towards an expense
func (h *ExpensePaymentHandler) CreateExpensePayment(c *gin.Context) {
	userID := c.GetInt("userID")
	expenseID, err := strconv.Atoi(c.Param("expenseId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid expense ID"})
		return
	}

	// Get the expense to verify it exists and user is a member of the group
	expense, err := db.GetExpenseByID(h.DB, expenseID)
	if err != nil {
		if err == db.ErrNotFound {
			c.JSON(http.StatusNotFound, models.APIResponse{Success: false, Error: "Expense not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to get expense"})
		return
	}

	// Verify user is a member of the group
	isMember, err := db.IsGroupMember(h.DB, expense.GroupID, userID)
	if err != nil {
		log.Printf("ERROR CreateExpensePayment: IsGroupMember group %d, user %d: %v", expense.GroupID, userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to verify group membership"})
		return
	}
	if !isMember {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "You are not a member of this group"})
		return
	}

	// Get what this user owes for this expense
	splits, err := db.GetExpenseSplits(h.DB, expenseID)
	if err != nil {
		log.Printf("ERROR CreateExpensePayment: GetExpenseSplits %d: %v", expenseID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to get expense splits"})
		return
	}

	var userOwes float64
	for _, split := range splits {
		if split.UserID == userID {
			userOwes = split.Amount
			break
		}
	}

	// If this user is the one who paid for the expense, they can't pay themselves
	if expense.PaidBy == userID {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "You paid for this expense, you cannot pay yourself back"})
		return
	}

	// Calculate how much they've already paid
	alreadyPaid, err := db.GetTotalPaymentsForExpense(h.DB, expenseID, userID)
	if err != nil {
		log.Printf("ERROR CreateExpensePayment: GetTotalPayments expense %d, user %d: %v", expenseID, userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to calculate existing payments"})
		return
	}

	var req models.CreateExpensePaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	// Check if this payment would exceed what they owe
	remaining := userOwes - alreadyPaid
	if req.Amount > remaining+0.01 { // Small tolerance for floating point
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Payment amount exceeds remaining debt. You owe " + strconv.FormatFloat(remaining, 'f', 2, 64),
		})
		return
	}

	payment, err := db.CreateExpensePayment(h.DB, expenseID, userID, req.Amount, req.Note)
	if err != nil {
		log.Printf("ERROR CreateExpensePayment: expense %d, user %d: %v", expenseID, userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to create payment"})
		return
	}

	// Log activity - note: expense.PaidBy is who originally paid, userID is who is paying back
	amount := req.Amount
	description := "Payment for: " + expense.Description
	db.LogActivity(h.DB, expense.GroupID, userID, db.ActivityPayment, description, &amount, &expense.PaidBy)

	c.JSON(http.StatusCreated, models.APIResponse{Success: true, Data: payment})
}

// GetExpensePayments returns all payments for an expense
func (h *ExpensePaymentHandler) GetExpensePayments(c *gin.Context) {
	userID := c.GetInt("userID")
	expenseID, err := strconv.Atoi(c.Param("expenseId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid expense ID"})
		return
	}

	// Get the expense to verify it exists and user is a member of the group
	expense, err := db.GetExpenseByID(h.DB, expenseID)
	if err != nil {
		if err == db.ErrNotFound {
			c.JSON(http.StatusNotFound, models.APIResponse{Success: false, Error: "Expense not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to get expense"})
		return
	}

	// Verify user is a member of the group
	isMember, err := db.IsGroupMember(h.DB, expense.GroupID, userID)
	if err != nil {
		log.Printf("ERROR GetExpensePayments: IsGroupMember group %d, user %d: %v", expense.GroupID, userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to verify group membership"})
		return
	}
	if !isMember {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "You are not a member of this group"})
		return
	}

	payments, err := db.GetExpensePayments(h.DB, expenseID)
	if err != nil {
		log.Printf("ERROR GetExpensePayments: expense %d: %v", expenseID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to get payments"})
		return
	}

	if payments == nil {
		payments = []models.ExpensePayment{}
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: payments})
}

// DeleteExpensePayment removes a payment record
func (h *ExpensePaymentHandler) DeleteExpensePayment(c *gin.Context) {
	userID := c.GetInt("userID")
	paymentID, err := strconv.Atoi(c.Param("paymentId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid payment ID"})
		return
	}

	// Get the payment
	payment, err := db.GetExpensePaymentByID(h.DB, paymentID)
	if err != nil {
		if err == db.ErrNotFound {
			c.JSON(http.StatusNotFound, models.APIResponse{Success: false, Error: "Payment not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to get payment"})
		return
	}

	// Only the person who made the payment can delete it
	if payment.PaidBy != userID {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "You can only delete your own payments"})
		return
	}

	if err := db.DeleteExpensePayment(h.DB, paymentID); err != nil {
		log.Printf("ERROR DeleteExpensePayment: payment %d: %v", paymentID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to delete payment"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Message: "Payment deleted"})
}

// GetGroupExpensePaymentStatuses returns payment status for all expenses in a group
func (h *ExpensePaymentHandler) GetGroupExpensePaymentStatuses(c *gin.Context) {
	userID := c.GetInt("userID")
	groupID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid group ID"})
		return
	}

	// Verify user is a member of the group
	isMember, err := db.IsGroupMember(h.DB, groupID, userID)
	if err != nil {
		log.Printf("ERROR GetGroupExpensePaymentStatuses: IsGroupMember group %d, user %d: %v", groupID, userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to verify group membership"})
		return
	}
	if !isMember {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "You are not a member of this group"})
		return
	}

	statuses, err := db.GetGroupExpensePaymentStatuses(h.DB, groupID)
	if err != nil {
		log.Printf("ERROR GetGroupExpensePaymentStatuses: group %d: %v", groupID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to get payment statuses"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: statuses})
}
