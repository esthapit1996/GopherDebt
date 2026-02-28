package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"gopherdebt/db"
	"gopherdebt/models"
)

type GroupHandler struct {
	DB *sql.DB
}

func NewGroupHandler(database *sql.DB) *GroupHandler {
	return &GroupHandler{DB: database}
}

func (h *GroupHandler) CreateGroup(c *gin.Context) {
	userID := c.GetInt("userID")
	var req models.CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	group, err := db.CreateGroup(h.DB, req.Name, req.Description, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to create group"})
		return
	}

	c.JSON(http.StatusCreated, models.APIResponse{Success: true, Message: "Group created successfully", Data: group})
}

func (h *GroupHandler) GetGroup(c *gin.Context) {
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

	group, err := db.GetGroupByID(h.DB, groupID)
	if err == db.ErrNotFound {
		c.JSON(http.StatusNotFound, models.APIResponse{Success: false, Error: "Group not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to fetch group"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: group})
}

func (h *GroupHandler) GetMyGroups(c *gin.Context) {
	userID := c.GetInt("userID")
	groups, err := db.GetUserGroups(h.DB, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to fetch groups"})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: groups})
}

func (h *GroupHandler) AddMember(c *gin.Context) {
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

	var req models.AddMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	_, err = db.GetUserByID(h.DB, req.UserID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.APIResponse{Success: false, Error: "User not found"})
		return
	}

	err = db.AddGroupMember(h.DB, groupID, req.UserID)
	if err != nil {
		c.JSON(http.StatusConflict, models.APIResponse{Success: false, Error: "User is already a member of this group"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Message: "Member added successfully"})
}

func (h *GroupHandler) RemoveMember(c *gin.Context) {
	userID := c.GetInt("userID")
	groupID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid group ID"})
		return
	}

	memberID, err := strconv.Atoi(c.Param("memberID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid member ID"})
		return
	}

	isMember, err := db.IsGroupMember(h.DB, groupID, userID)
	if err != nil || !isMember {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "You are not a member of this group"})
		return
	}

	err = db.RemoveGroupMember(h.DB, groupID, memberID)
	if err == db.ErrNotFound {
		c.JSON(http.StatusNotFound, models.APIResponse{Success: false, Error: "Member not found in group"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to remove member"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Message: "Member removed successfully"})
}

func (h *GroupHandler) DeleteGroup(c *gin.Context) {
	userID := c.GetInt("userID")
	groupID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid group ID"})
		return
	}

	group, err := db.GetGroupByID(h.DB, groupID)
	if err == db.ErrNotFound {
		c.JSON(http.StatusNotFound, models.APIResponse{Success: false, Error: "Group not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to fetch group"})
		return
	}

	if group.CreatedBy != userID {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "Only the group creator can delete the group"})
		return
	}

	err = db.DeleteGroup(h.DB, groupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to delete group"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Message: "Group deleted successfully"})
}
