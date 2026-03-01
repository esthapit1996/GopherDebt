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

type ExpenseHandler struct {
	DB *sql.DB
}

func NewExpenseHandler(database *sql.DB) *ExpenseHandler {
	return &ExpenseHandler{DB: database}
}

func (h *ExpenseHandler) CreateExpense(c *gin.Context) {
	userID := c.GetInt("userID")
	groupID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid group ID"})
		return
	}

	isMember, err := db.IsGroupMember(h.DB, groupID, userID)
	if err != nil {
		log.Printf("ERROR CreateExpense: IsGroupMember group %d, user %d: %v", groupID, userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to verify group membership"})
		return
	}
	if !isMember {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "You are not a member of this group"})
		return
	}

	var req models.CreateExpenseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	var splits []models.ExpenseSplitInput

	switch req.SplitType {
	case "equal":
		members, err := db.GetGroupMembers(h.DB, groupID)
		if err != nil {
			log.Printf("ERROR CreateExpense: GetGroupMembers for group %d: %v", groupID, err)
			c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to get group members"})
			return
		}
		splitAmount := req.Amount / float64(len(members))
		for _, member := range members {
			splits = append(splits, models.ExpenseSplitInput{UserID: member.ID, Amount: splitAmount})
		}

	case "exact":
		if len(req.SplitWith) == 0 {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "split_with is required for exact split type"})
			return
		}
		var total float64
		for _, s := range req.SplitWith {
			total += s.Amount
		}
		if total < req.Amount-0.01 || total > req.Amount+0.01 {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Split amounts must equal the expense amount"})
			return
		}
		splits = req.SplitWith

	case "percentage":
		if len(req.SplitWith) == 0 {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "split_with is required for percentage split type"})
			return
		}
		var totalPercent float64
		for _, s := range req.SplitWith {
			totalPercent += s.Amount
		}
		if totalPercent < 99.99 || totalPercent > 100.01 {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Percentages must total 100"})
			return
		}
		for _, s := range req.SplitWith {
			splits = append(splits, models.ExpenseSplitInput{UserID: s.UserID, Amount: req.Amount * s.Amount / 100})
		}

	default:
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid split type"})
		return
	}

	for _, split := range splits {
		isMember, err := db.IsGroupMember(h.DB, groupID, split.UserID)
		if err != nil {
			log.Printf("ERROR CreateExpense: IsGroupMember split user %d, group %d: %v", split.UserID, groupID, err)
			c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to verify split member"})
			return
		}
		if !isMember {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "All users in split must be group members"})
			return
		}
	}

	expense, err := db.CreateExpense(h.DB, groupID, userID, req.Amount, req.Description, req.SplitType, splits)
	if err != nil {
		log.Printf("ERROR CreateExpense: group %d, user %d: %v", groupID, userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to create expense"})
		return
	}

	// Log activity
	amount := req.Amount
	db.LogActivity(h.DB, groupID, userID, db.ActivityExpenseCreated, req.Description, &amount, nil)

	expense, _ = db.GetExpenseByID(h.DB, expense.ID)
	c.JSON(http.StatusCreated, models.APIResponse{Success: true, Message: "Expense created successfully", Data: expense})
}

func (h *ExpenseHandler) GetGroupExpenses(c *gin.Context) {
	userID := c.GetInt("userID")
	groupID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid group ID"})
		return
	}

	isMember, err := db.IsGroupMember(h.DB, groupID, userID)
	if err != nil {
		log.Printf("ERROR GetGroupExpenses: IsGroupMember group %d, user %d: %v", groupID, userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to verify group membership"})
		return
	}
	if !isMember {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "You are not a member of this group"})
		return
	}

	expenses, err := db.GetGroupExpenses(h.DB, groupID)
	if err != nil {
		log.Printf("ERROR GetGroupExpenses: group %d: %v", groupID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to fetch expenses"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: expenses})
}

func (h *ExpenseHandler) GetExpense(c *gin.Context) {
	userID := c.GetInt("userID")
	groupID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid group ID"})
		return
	}

	expenseID, err := strconv.Atoi(c.Param("expenseID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid expense ID"})
		return
	}

	isMember, err := db.IsGroupMember(h.DB, groupID, userID)
	if err != nil {
		log.Printf("ERROR GetExpense: IsGroupMember group %d, user %d: %v", groupID, userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to verify group membership"})
		return
	}
	if !isMember {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "You are not a member of this group"})
		return
	}

	expense, err := db.GetExpenseByID(h.DB, expenseID)
	if err == db.ErrNotFound {
		c.JSON(http.StatusNotFound, models.APIResponse{Success: false, Error: "Expense not found"})
		return
	}
	if err != nil {
		log.Printf("ERROR GetExpense: expense %d: %v", expenseID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to fetch expense"})
		return
	}

	if expense.GroupID != groupID {
		c.JSON(http.StatusNotFound, models.APIResponse{Success: false, Error: "Expense not found in this group"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: expense})
}

func (h *ExpenseHandler) DeleteExpense(c *gin.Context) {
	userID := c.GetInt("userID")
	groupID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid group ID"})
		return
	}

	expenseID, err := strconv.Atoi(c.Param("expenseID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid expense ID"})
		return
	}

	isMember, err := db.IsGroupMember(h.DB, groupID, userID)
	if err != nil {
		log.Printf("ERROR DeleteExpense: IsGroupMember group %d, user %d: %v", groupID, userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to verify group membership"})
		return
	}
	if !isMember {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "You are not a member of this group"})
		return
	}

	expense, err := db.GetExpenseByID(h.DB, expenseID)
	if err == db.ErrNotFound {
		c.JSON(http.StatusNotFound, models.APIResponse{Success: false, Error: "Expense not found"})
		return
	}
	if err != nil {
		log.Printf("ERROR DeleteExpense: GetExpenseByID %d: %v", expenseID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to fetch expense"})
		return
	}

	if expense.GroupID != groupID {
		c.JSON(http.StatusNotFound, models.APIResponse{Success: false, Error: "Expense not found in this group"})
		return
	}

	err = db.DeleteExpense(h.DB, expenseID)
	if err != nil {
		log.Printf("ERROR DeleteExpense: DeleteExpense %d: %v", expenseID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to delete expense"})
		return
	}

	// Log activity
	db.LogActivity(h.DB, groupID, userID, db.ActivityExpenseDeleted, expense.Description, &expense.Amount, nil)

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Message: "Expense deleted successfully"})
}
