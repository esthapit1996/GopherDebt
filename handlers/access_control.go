package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"gopherdebt/db"
	"gopherdebt/models"
)

type AccessControlHandler struct {
	DB *sql.DB
}

func NewAccessControlHandler(database *sql.DB) *AccessControlHandler {
	return &AccessControlHandler{DB: database}
}

type AddEmailRequest struct {
	Email  string `json:"email" binding:"required,email"`
	Reason string `json:"reason"` // Only used for blacklist
}

// GetWhitelist returns all whitelisted emails
func (h *AccessControlHandler) GetWhitelist(c *gin.Context) {
	// Verify requester is founder
	requesterID := c.GetInt("userID")
	requester, err := db.GetUserByID(h.DB, requesterID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to verify requester"})
		return
	}
	if requester.Email != db.FounderEmail {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "Only the founder can view this"})
		return
	}

	entries, err := db.GetWhitelist(h.DB)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to fetch whitelist"})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: entries})
}

// GetBlacklist returns all blacklisted emails
func (h *AccessControlHandler) GetBlacklist(c *gin.Context) {
	// Verify requester is founder
	requesterID := c.GetInt("userID")
	requester, err := db.GetUserByID(h.DB, requesterID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to verify requester"})
		return
	}
	if requester.Email != db.FounderEmail {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "Only the founder can view this"})
		return
	}

	entries, err := db.GetBlacklist(h.DB)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to fetch blacklist"})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: entries})
}

// AddToWhitelist adds an email to the whitelist
func (h *AccessControlHandler) AddToWhitelist(c *gin.Context) {
	// Verify requester is founder
	requesterID := c.GetInt("userID")
	requester, err := db.GetUserByID(h.DB, requesterID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to verify requester"})
		return
	}
	if requester.Email != db.FounderEmail {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "Only the founder can modify this"})
		return
	}

	var req AddEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	entry, err := db.AddToWhitelist(h.DB, req.Email, requesterID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to add email - it may already exist"})
		return
	}
	c.JSON(http.StatusCreated, models.APIResponse{Success: true, Data: entry})
}

// AddToBlacklist adds an email to the blacklist
func (h *AccessControlHandler) AddToBlacklist(c *gin.Context) {
	// Verify requester is founder
	requesterID := c.GetInt("userID")
	requester, err := db.GetUserByID(h.DB, requesterID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to verify requester"})
		return
	}
	if requester.Email != db.FounderEmail {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "Only the founder can modify this"})
		return
	}

	var req AddEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	// Cannot blacklist the founder
	if req.Email == db.FounderEmail {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "Cannot blacklist the founder"})
		return
	}

	entry, err := db.AddToBlacklist(h.DB, req.Email, req.Reason, requesterID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to add email - it may already exist"})
		return
	}
	c.JSON(http.StatusCreated, models.APIResponse{Success: true, Data: entry})
}

// RemoveFromWhitelist removes an email from the whitelist
func (h *AccessControlHandler) RemoveFromWhitelist(c *gin.Context) {
	// Verify requester is founder
	requesterID := c.GetInt("userID")
	requester, err := db.GetUserByID(h.DB, requesterID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to verify requester"})
		return
	}
	if requester.Email != db.FounderEmail {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "Only the founder can modify this"})
		return
	}

	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid ID"})
		return
	}

	if err := db.RemoveFromWhitelist(h.DB, id); err != nil {
		if err == db.ErrNotFound {
			c.JSON(http.StatusNotFound, models.APIResponse{Success: false, Error: "Entry not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to remove entry"})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: "Removed successfully"})
}

// RemoveFromBlacklist removes an email from the blacklist
func (h *AccessControlHandler) RemoveFromBlacklist(c *gin.Context) {
	// Verify requester is founder
	requesterID := c.GetInt("userID")
	requester, err := db.GetUserByID(h.DB, requesterID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to verify requester"})
		return
	}
	if requester.Email != db.FounderEmail {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "Only the founder can modify this"})
		return
	}

	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid ID"})
		return
	}

	if err := db.RemoveFromBlacklist(h.DB, id); err != nil {
		if err == db.ErrNotFound {
			c.JSON(http.StatusNotFound, models.APIResponse{Success: false, Error: "Entry not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to remove entry"})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: "Removed successfully"})
}
