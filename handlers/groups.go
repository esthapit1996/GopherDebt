package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"strings"

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

	group, err := db.CreateGroup(h.DB, req.Name, req.Description, req.Emoji, userID)
	if err != nil {
		log.Printf("ERROR CreateGroup: user %d: %v", userID, err)
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
	if err != nil {
		log.Printf("ERROR GetGroup: IsGroupMember group %d, user %d: %v", groupID, userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to verify group membership"})
		return
	}
	if !isMember {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "You are not a member of this group"})
		return
	}

	group, err := db.GetGroupByID(h.DB, groupID)
	if err == db.ErrNotFound {
		c.JSON(http.StatusNotFound, models.APIResponse{Success: false, Error: "Group not found"})
		return
	}
	if err != nil {
		log.Printf("ERROR GetGroup: GetGroupByID %d: %v", groupID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to fetch group"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: group})
}

func (h *GroupHandler) GetMyGroups(c *gin.Context) {
	userID := c.GetInt("userID")
	groups, err := db.GetUserGroups(h.DB, userID)
	if err != nil {
		log.Printf("ERROR GetMyGroups: user %d: %v", userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to fetch groups"})
		return
	}

	// Build response with balance info for each group
	result := make([]models.GroupWithBalance, len(groups))
	for i, group := range groups {
		balance, _ := db.GetUserBalanceInGroup(h.DB, group.ID, userID)
		result[i] = models.GroupWithBalance{
			Group:     group,
			MyBalance: balance,
		}
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: result})
}

func (h *GroupHandler) AddMember(c *gin.Context) {
	userID := c.GetInt("userID")
	groupID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid group ID"})
		return
	}

	isMember, err := db.IsGroupMember(h.DB, groupID, userID)
	if err != nil {
		log.Printf("ERROR AddMember: IsGroupMember group %d, user %d: %v", groupID, userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to verify group membership"})
		return
	}
	if !isMember {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "You are not a member of this group"})
		return
	}

	var req models.AddMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	_, err = db.GetUserByID(h.DB, req.UserID)
	if err == db.ErrNotFound {
		c.JSON(http.StatusNotFound, models.APIResponse{Success: false, Error: "User not found"})
		return
	}
	if err != nil {
		log.Printf("ERROR AddMember: GetUserByID %d: %v", req.UserID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to look up user"})
		return
	}

	err = db.AddGroupMember(h.DB, groupID, req.UserID)
	if err != nil {
		c.JSON(http.StatusConflict, models.APIResponse{Success: false, Error: "User is already a member of this group"})
		return
	}

	// Log activity
	db.LogActivity(h.DB, groupID, userID, db.ActivityMemberAdded, "Added member", nil, &req.UserID)

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
	if err != nil {
		log.Printf("ERROR RemoveMember: IsGroupMember group %d, user %d: %v", groupID, userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to verify group membership"})
		return
	}
	if !isMember {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "You are not a member of this group"})
		return
	}

	err = db.RemoveGroupMember(h.DB, groupID, memberID)
	if err == db.ErrNotFound {
		c.JSON(http.StatusNotFound, models.APIResponse{Success: false, Error: "Member not found in group"})
		return
	}
	if err != nil {
		log.Printf("ERROR RemoveMember: group %d, member %d: %v", groupID, memberID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to remove member"})
		return
	}

	// Log activity
	db.LogActivity(h.DB, groupID, userID, db.ActivityMemberRemoved, "Removed member", nil, &memberID)

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Message: "Member removed successfully"})
}

func (h *GroupHandler) UpdateGroup(c *gin.Context) {
	userID := c.GetInt("userID")
	groupID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid group ID"})
		return
	}

	isMember, err := db.IsGroupMember(h.DB, groupID, userID)
	if err != nil {
		log.Printf("ERROR UpdateGroup: IsGroupMember group %d, user %d: %v", groupID, userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to verify group membership"})
		return
	}
	if !isMember {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "You are not a member of this group"})
		return
	}

	var req models.UpdateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	// Get old group to build activity description
	oldGroup, err := db.GetGroupByID(h.DB, groupID)
	if err == db.ErrNotFound {
		c.JSON(http.StatusNotFound, models.APIResponse{Success: false, Error: "Group not found"})
		return
	}
	if err != nil {
		log.Printf("ERROR UpdateGroup: GetGroupByID %d: %v", groupID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to fetch group"})
		return
	}

	group, err := db.UpdateGroup(h.DB, groupID, req.Name, req.Description)
	if err == db.ErrNotFound {
		c.JSON(http.StatusNotFound, models.APIResponse{Success: false, Error: "Group not found"})
		return
	}
	if err != nil {
		log.Printf("ERROR UpdateGroup: group %d: %v", groupID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to update group"})
		return
	}

	// Build activity description
	var changes []string
	if oldGroup.Name != req.Name {
		changes = append(changes, "name: \""+oldGroup.Name+"\" → \""+req.Name+"\"")
	}
	if oldGroup.Description != req.Description {
		changes = append(changes, "description updated")
	}
	if len(changes) > 0 {
		desc := "Updated group: " + strings.Join(changes, ", ")
		db.LogActivity(h.DB, groupID, userID, db.ActivityGroupUpdated, desc, nil, nil)
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Message: "Group updated successfully", Data: group})
}

func (h *GroupHandler) DeleteGroup(c *gin.Context) {
	userID := c.GetInt("userID")
	groupID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid group ID"})
		return
	}

	// Check if user is a member of this group
	isMember, err := db.IsGroupMember(h.DB, groupID, userID)
	if err != nil {
		log.Printf("ERROR DeleteGroup: IsGroupMember group %d, user %d: %v", groupID, userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to verify group membership"})
		return
	}
	if !isMember {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "You are not a member of this group"})
		return
	}

	_, err = db.GetGroupByID(h.DB, groupID)
	if err == db.ErrNotFound {
		c.JSON(http.StatusNotFound, models.APIResponse{Success: false, Error: "Group not found"})
		return
	}
	if err != nil {
		log.Printf("ERROR DeleteGroup: GetGroupByID %d: %v", groupID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to fetch group"})
		return
	}

	// Check if all balances are settled
	isSettled, err := db.IsGroupSettled(h.DB, groupID)
	if err != nil {
		log.Printf("ERROR DeleteGroup: IsGroupSettled %d: %v", groupID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to check balances"})
		return
	}
	if !isSettled {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Cannot delete group with unsettled balances"})
		return
	}

	err = db.DeleteGroup(h.DB, groupID)
	if err != nil {
		log.Printf("ERROR DeleteGroup: DeleteGroup %d: %v", groupID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to delete group"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Message: "Group deleted successfully"})
}
