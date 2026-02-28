package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"gopherdebt/db"
	"gopherdebt/models"
)

type ActivityHandler struct {
	DB *sql.DB
}

func NewActivityHandler(database *sql.DB) *ActivityHandler {
	return &ActivityHandler{DB: database}
}

// GetGroupActivities returns the activity history for a group
func (h *ActivityHandler) GetGroupActivities(c *gin.Context) {
	userID := c.GetInt("userID")
	groupID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid group ID"})
		return
	}

	// Verify user is a member of the group
	isMember, err := db.IsGroupMember(h.DB, groupID, userID)
	if err != nil || !isMember {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "You are not a member of this group"})
		return
	}

	// Get limit from query param, default 50
	limit := 50
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	activities, err := db.GetGroupActivities(h.DB, groupID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to fetch activities"})
		return
	}

	if activities == nil {
		activities = []models.Activity{}
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: activities})
}
