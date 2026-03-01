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

type SettlementHandler struct {
	DB *sql.DB
}

func NewSettlementHandler(database *sql.DB) *SettlementHandler {
	return &SettlementHandler{DB: database}
}

func (h *SettlementHandler) CreateSettlement(c *gin.Context) {
	userID := c.GetInt("userID")
	groupID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid group ID"})
		return
	}

	isMember, err := db.IsGroupMember(h.DB, groupID, userID)
	if err != nil || !isMember {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "You are not a member of this group"})
		return
	}

	var req models.CreateSettlementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	isMember, err = db.IsGroupMember(h.DB, groupID, req.PaidTo)
	if err != nil || !isMember {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Recipient must be a group member"})
		return
	}

	if userID == req.PaidTo {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Cannot settle with yourself"})
		return
	}

	settlement, err := db.CreateSettlement(h.DB, groupID, userID, req.PaidTo, req.Amount)
	if err != nil {
		log.Printf("ERROR CreateSettlement: group %d, user %d -> %d: %v", groupID, userID, req.PaidTo, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to create settlement"})
		return
	}

	// Log activity
	amount := req.Amount
	db.LogActivity(h.DB, groupID, userID, db.ActivitySettlement, "Settled up", &amount, &req.PaidTo)

	c.JSON(http.StatusCreated, models.APIResponse{Success: true, Message: "Settlement recorded successfully", Data: settlement})
}

func (h *SettlementHandler) GetGroupSettlements(c *gin.Context) {
	userID := c.GetInt("userID")
	groupID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid group ID"})
		return
	}

	isMember, err := db.IsGroupMember(h.DB, groupID, userID)
	if err != nil || !isMember {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "You are not a member of this group"})
		return
	}

	settlements, err := db.GetGroupSettlements(h.DB, groupID)
	if err != nil {
		log.Printf("ERROR GetGroupSettlements: group %d: %v", groupID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to fetch settlements"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: settlements})
}

func (h *SettlementHandler) GetGroupBalances(c *gin.Context) {
	userID := c.GetInt("userID")
	groupID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid group ID"})
		return
	}

	isMember, err := db.IsGroupMember(h.DB, groupID, userID)
	if err != nil || !isMember {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "You are not a member of this group"})
		return
	}

	balances, err := db.CalculateGroupBalances(h.DB, groupID)
	if err != nil {
		log.Printf("ERROR GetGroupBalances: group %d: %v", groupID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to calculate balances"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: models.GroupBalance{GroupID: groupID, Balances: balances}})
}

func (h *SettlementHandler) GetMyBalance(c *gin.Context) {
	userID := c.GetInt("userID")
	groupID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid group ID"})
		return
	}

	isMember, err := db.IsGroupMember(h.DB, groupID, userID)
	if err != nil || !isMember {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "You are not a member of this group"})
		return
	}

	balance, err := db.GetUserBalanceInGroup(h.DB, groupID, userID)
	if err != nil {
		log.Printf("ERROR GetMyBalance: group %d, user %d: %v", groupID, userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to calculate balance"})
		return
	}

	user, _ := db.GetUserByID(h.DB, userID)
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: models.UserBalance{User: user, Balance: balance}})
}
