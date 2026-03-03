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

type StashHandler struct {
	DB *sql.DB
}

func NewStashHandler(database *sql.DB) *StashHandler {
	return &StashHandler{DB: database}
}

// GetStashExpenses returns all personal expenses for the authenticated user
func (h *StashHandler) GetStashExpenses(c *gin.Context) {
	userID := c.GetInt("userID")

	expenses, err := db.GetStashExpenses(h.DB, userID)
	if err != nil {
		log.Printf("ERROR GetStashExpenses: user %d: %v", userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to load stash expenses",
		})
		return
	}

	if expenses == nil {
		expenses = []models.StashExpense{}
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    expenses,
	})
}

// CreateStashExpense adds a personal expense
func (h *StashHandler) CreateStashExpense(c *gin.Context) {
	userID := c.GetInt("userID")

	var req models.CreateStashExpenseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Invalid request: " + err.Error(),
		})
		return
	}

	expense, err := db.CreateStashExpense(h.DB, userID, req.Amount, req.Description, req.Category)
	if err != nil {
		log.Printf("ERROR CreateStashExpense: user %d: %v", userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to add expense",
		})
		return
	}

	c.JSON(http.StatusCreated, models.APIResponse{
		Success: true,
		Data:    expense,
	})
}

// DeleteStashExpense removes a personal expense
func (h *StashHandler) DeleteStashExpense(c *gin.Context) {
	userID := c.GetInt("userID")
	expenseID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Invalid expense ID",
		})
		return
	}

	if err := db.DeleteStashExpense(h.DB, expenseID, userID); err != nil {
		if err == db.ErrNotFound {
			c.JSON(http.StatusNotFound, models.APIResponse{
				Success: false,
				Error:   "Expense not found",
			})
			return
		}
		log.Printf("ERROR DeleteStashExpense: user %d, expense %d: %v", userID, expenseID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to delete expense",
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Expense deleted",
	})
}

// GetStashSummary returns total spent and breakdown by category
func (h *StashHandler) GetStashSummary(c *gin.Context) {
	userID := c.GetInt("userID")

	summary, err := db.GetStashSummary(h.DB, userID)
	if err != nil {
		log.Printf("ERROR GetStashSummary: user %d: %v", userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to load summary",
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    summary,
	})
}

// ClearStashExpenses removes all personal expenses
func (h *StashHandler) ClearStashExpenses(c *gin.Context) {
	userID := c.GetInt("userID")

	if err := db.ClearStashExpenses(h.DB, userID); err != nil {
		log.Printf("ERROR ClearStashExpenses: user %d: %v", userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to clear expenses",
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "All stash expenses cleared",
	})
}
