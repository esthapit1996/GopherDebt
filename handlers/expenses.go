package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"math"
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

	// Fetch group members once — used for equal splits AND membership validation
	groupMembers, err := db.GetGroupMembers(h.DB, groupID)
	if err != nil {
		log.Printf("ERROR CreateExpense: GetGroupMembers for group %d: %v", groupID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to get group members"})
		return
	}
	memberSet := make(map[int]bool, len(groupMembers))
	for _, m := range groupMembers {
		memberSet[m.ID] = true
	}

	switch req.SplitType {
	case "equal":
		// Round each share to 2 decimal places; give rounding remainder to payer
		n := len(groupMembers)
		perPerson := math.Floor(req.Amount/float64(n)*100) / 100
		assigned := perPerson * float64(n)
		remainder := math.Round((req.Amount-assigned)*100) / 100

		// Determine who the payer is
		payerID := userID
		if req.PaidBy > 0 {
			payerID = req.PaidBy
		}

		for _, member := range groupMembers {
			amt := perPerson
			if member.ID == payerID {
				amt = math.Round((perPerson+remainder)*100) / 100
			}
			splits = append(splits, models.ExpenseSplitInput{UserID: member.ID, Amount: amt})
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
			amt := math.Round(req.Amount*s.Amount/100*100) / 100
			splits = append(splits, models.ExpenseSplitInput{UserID: s.UserID, Amount: amt})
		}
		// Fix rounding: adjust payer's share so total matches exactly
		var splitTotal float64
		for _, s := range splits {
			splitTotal += s.Amount
		}
		if diff := math.Round((req.Amount-splitTotal)*100) / 100; diff != 0 {
			payerID := userID
			if req.PaidBy > 0 {
				payerID = req.PaidBy
			}
			for i, s := range splits {
				if s.UserID == payerID {
					splits[i].Amount = math.Round((s.Amount+diff)*100) / 100
					break
				}
			}
		}

	default:
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid split type"})
		return
	}

	// Validate all split users are group members (in-memory check)
	for _, split := range splits {
		if !memberSet[split.UserID] {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "All users in split must be group members"})
			return
		}
	}

	// Determine who paid: use req.PaidBy if provided, otherwise the logged-in user
	payer := userID
	// Verify the payer is a group member (use memberSet already built)
	if req.PaidBy > 0 {
		if !memberSet[req.PaidBy] {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Payer must be a group member"})
			return
		}
		payer = req.PaidBy
	}

	expense, err := db.CreateExpense(h.DB, groupID, payer, req.Amount, req.Description, req.SplitType, splits)
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

// GetUnpaidExpenses returns only expenses where the current user still owes money
func (h *ExpenseHandler) GetUnpaidExpenses(c *gin.Context) {
	userID := c.GetInt("userID")
	groupID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid group ID"})
		return
	}

	isMember, err := db.IsGroupMember(h.DB, groupID, userID)
	if err != nil {
		log.Printf("ERROR GetUnpaidExpenses: IsGroupMember group %d, user %d: %v", groupID, userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to verify group membership"})
		return
	}
	if !isMember {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "You are not a member of this group"})
		return
	}

	expenses, err := db.GetUnpaidExpensesForUser(h.DB, groupID, userID)
	if err != nil {
		log.Printf("ERROR GetUnpaidExpenses: group %d, user %d: %v", groupID, userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to fetch unpaid expenses"})
		return
	}

	if expenses == nil {
		expenses = []models.Expense{}
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

func (h *ExpenseHandler) UpdateExpense(c *gin.Context) {
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
		log.Printf("ERROR UpdateExpense: IsGroupMember group %d, user %d: %v", groupID, userID, err)
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
		log.Printf("ERROR UpdateExpense: GetExpenseByID %d: %v", expenseID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to fetch expense"})
		return
	}

	if expense.GroupID != groupID {
		c.JSON(http.StatusNotFound, models.APIResponse{Success: false, Error: "Expense not found in this group"})
		return
	}

	var req models.CreateExpenseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	// Fetch group members for validation
	groupMembers, err := db.GetGroupMembers(h.DB, groupID)
	if err != nil {
		log.Printf("ERROR UpdateExpense: GetGroupMembers for group %d: %v", groupID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to get group members"})
		return
	}
	memberSet := make(map[int]bool, len(groupMembers))
	for _, m := range groupMembers {
		memberSet[m.ID] = true
	}

	var splits []models.ExpenseSplitInput
	switch req.SplitType {
	case "equal":
		n := len(groupMembers)
		perPerson := math.Floor(req.Amount/float64(n)*100) / 100
		assigned := perPerson * float64(n)
		remainder := math.Round((req.Amount-assigned)*100) / 100

		payerID := userID
		if req.PaidBy > 0 {
			payerID = req.PaidBy
		}

		for _, member := range groupMembers {
			amt := perPerson
			if member.ID == payerID {
				amt = math.Round((perPerson+remainder)*100) / 100
			}
			splits = append(splits, models.ExpenseSplitInput{UserID: member.ID, Amount: amt})
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
			amt := math.Round(req.Amount*s.Amount/100*100) / 100
			splits = append(splits, models.ExpenseSplitInput{UserID: s.UserID, Amount: amt})
		}
		var splitTotal float64
		for _, s := range splits {
			splitTotal += s.Amount
		}
		if diff := math.Round((req.Amount-splitTotal)*100) / 100; diff != 0 {
			payerID := userID
			if req.PaidBy > 0 {
				payerID = req.PaidBy
			}
			for i, s := range splits {
				if s.UserID == payerID {
					splits[i].Amount = math.Round((s.Amount+diff)*100) / 100
					break
				}
			}
		}

	default:
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid split type"})
		return
	}

	// Validate split users
	for _, split := range splits {
		if !memberSet[split.UserID] {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "All users in split must be group members"})
			return
		}
	}

	payer := userID
	if req.PaidBy > 0 {
		if !memberSet[req.PaidBy] {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Payer must be a group member"})
			return
		}
		payer = req.PaidBy
	}

	if err := db.UpdateExpense(h.DB, expenseID, payer, req.Amount, req.Description, req.SplitType, splits); err != nil {
		if err == db.ErrNotFound {
			c.JSON(http.StatusNotFound, models.APIResponse{Success: false, Error: "Expense not found"})
			return
		}
		log.Printf("ERROR UpdateExpense: UpdateExpense %d: %v", expenseID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to update expense"})
		return
	}

	// Log activity
	amount := req.Amount
	db.LogActivity(h.DB, groupID, userID, db.ActivityExpenseUpdated, req.Description, &amount, nil)

	updated, _ := db.GetExpenseByID(h.DB, expenseID)
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Message: "Expense updated successfully", Data: updated})
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

func (h *ExpenseHandler) ClearAllExpenses(c *gin.Context) {
	userID := c.GetInt("userID")
	groupID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid group ID"})
		return
	}

	isMember, err := db.IsGroupMember(h.DB, groupID, userID)
	if err != nil {
		log.Printf("ERROR ClearAllExpenses: IsGroupMember group %d, user %d: %v", groupID, userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to verify group membership"})
		return
	}
	if !isMember {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "You are not a member of this group"})
		return
	}

	deleted, err := db.DeleteAllGroupExpenses(h.DB, groupID)
	if err != nil {
		log.Printf("ERROR ClearAllExpenses: DeleteAllGroupExpenses group %d: %v", groupID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to clear expenses"})
		return
	}

	desc := fmt.Sprintf("Cleared %d expenses", deleted)
	db.LogActivity(h.DB, groupID, userID, db.ActivityExpenseDeleted, desc, nil, nil)

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Message: fmt.Sprintf("Cleared %d expenses", deleted)})
}
