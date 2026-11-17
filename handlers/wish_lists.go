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

// WishListHandler serves shared GoWish lists. Every member can edit the list,
// add/remove members and add/edit/delete items. The last member leaving deletes the list.
type WishListHandler struct {
	DB *sql.DB
}

func NewWishListHandler(database *sql.DB) *WishListHandler {
	return &WishListHandler{DB: database}
}

func wishFail(c *gin.Context, status int, msg string) {
	c.JSON(status, models.APIResponse{Success: false, Error: msg})
}

// paramID parses a numeric path param, writing a 400 on failure
func paramID(c *gin.Context, param, label string) (int, bool) {
	id, err := strconv.Atoi(c.Param(param))
	if err != nil || id <= 0 {
		wishFail(c, http.StatusBadRequest, "Invalid "+label+" ID")
		return 0, false
	}
	return id, true
}

// memberListID parses :id and checks the caller belongs to the list.
// Non-members get 404 so list IDs can't be probed.
func (h *WishListHandler) memberListID(c *gin.Context) (int, bool) {
	list_id, ok := paramID(c, "id", "wish list")
	if !ok {
		return 0, false
	}
	user_id := c.GetInt("userID")
	is_member, err := db.IsWishListMember(h.DB, list_id, user_id)
	if err != nil {
		log.Printf("ERROR IsWishListMember: list %d, user %d: %v", list_id, user_id, err)
		wishFail(c, http.StatusInternalServerError, "Failed to verify wish list membership")
		return 0, false
	}
	if !is_member {
		wishFail(c, http.StatusNotFound, "Wish list not found")
		return 0, false
	}
	return list_id, true
}

// cleanWishList trims a list payload; a blank name is rejected with a 400
func cleanWishList(c *gin.Context, req *models.WishListRequest) bool {
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	req.Emoji = strings.TrimSpace(req.Emoji)
	if req.Name == "" {
		wishFail(c, http.StatusBadRequest, "Name is required")
		return false
	}
	return true
}

// bindWishList parses and validates an update payload, writing a 400 on failure
func bindWishList(c *gin.Context) (*models.WishListRequest, bool) {
	var req models.WishListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		wishFail(c, http.StatusBadRequest, "Invalid request: "+err.Error())
		return nil, false
	}
	return &req, cleanWishList(c, &req)
}

// bindCreateWishList parses and validates a create payload, writing a 400 on failure
func bindCreateWishList(c *gin.Context) (*models.CreateWishListRequest, bool) {
	var req models.CreateWishListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		wishFail(c, http.StatusBadRequest, "Invalid request: "+err.Error())
		return nil, false
	}
	return &req, cleanWishList(c, &req.WishListRequest)
}

// GetWishLists returns the shared lists the caller belongs to
func (h *WishListHandler) GetWishLists(c *gin.Context) {
	user_id := c.GetInt("userID")
	lists, err := db.GetUserWishLists(h.DB, user_id)
	if err != nil {
		log.Printf("ERROR GetUserWishLists: user %d: %v", user_id, err)
		wishFail(c, http.StatusInternalServerError, "Failed to load shared wish lists")
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: lists})
}

// CreateWishList creates a shared list with the caller and any member_ids as members
func (h *WishListHandler) CreateWishList(c *gin.Context) {
	user_id := c.GetInt("userID")
	req, ok := bindCreateWishList(c)
	if !ok {
		return
	}
	list, err := db.CreateWishList(h.DB, user_id, req.Name, req.Description, req.Emoji, req.MemberIDs)
	if err != nil {
		log.Printf("ERROR CreateWishList: user %d: %v", user_id, err)
		wishFail(c, http.StatusInternalServerError, "Failed to create wish list")
		return
	}
	c.JSON(http.StatusCreated, models.APIResponse{Success: true, Data: list})
}

// GetWishList returns one shared list with its members
func (h *WishListHandler) GetWishList(c *gin.Context) {
	list_id, ok := h.memberListID(c)
	if !ok {
		return
	}
	h.respondList(c, list_id)
}

// UpdateWishList changes a shared list's name, description and icon
func (h *WishListHandler) UpdateWishList(c *gin.Context) {
	list_id, ok := h.memberListID(c)
	if !ok {
		return
	}
	req, ok := bindWishList(c)
	if !ok {
		return
	}
	if err := db.UpdateWishList(h.DB, list_id, req.Name, req.Description, req.Emoji); err != nil {
		if err == db.ErrNotFound {
			wishFail(c, http.StatusNotFound, "Wish list not found")
			return
		}
		log.Printf("ERROR UpdateWishList: list %d: %v", list_id, err)
		wishFail(c, http.StatusInternalServerError, "Failed to update wish list")
		return
	}
	h.respondList(c, list_id)
}

// AddMember adds a user to a shared list
func (h *WishListHandler) AddMember(c *gin.Context) {
	list_id, ok := h.memberListID(c)
	if !ok {
		return
	}
	var req models.WishListMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		wishFail(c, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	if _, err := db.GetUserByID(h.DB, req.UserID); err != nil {
		if err == db.ErrNotFound {
			wishFail(c, http.StatusNotFound, "User not found")
			return
		}
		log.Printf("ERROR AddWishListMember: GetUserByID %d: %v", req.UserID, err)
		wishFail(c, http.StatusInternalServerError, "Failed to look up user")
		return
	}

	if err := db.AddWishListMember(h.DB, list_id, req.UserID); err != nil {
		if err == db.ErrAlreadyMember {
			wishFail(c, http.StatusConflict, "User is already a member of this wish list")
			return
		}
		log.Printf("ERROR AddWishListMember: list %d, user %d: %v", list_id, req.UserID, err)
		wishFail(c, http.StatusInternalServerError, "Failed to add member")
		return
	}
	h.respondList(c, list_id)
}

// RemoveMember removes a member (or the caller, i.e. leave). Deletes the list when it empties.
func (h *WishListHandler) RemoveMember(c *gin.Context) {
	member_id, ok := paramID(c, "userId", "member")
	if !ok {
		return
	}
	list_id, ok := h.memberListID(c)
	if !ok {
		return
	}
	deleted, err := db.RemoveWishListMember(h.DB, list_id, member_id)
	if err != nil {
		if err == db.ErrNotFound {
			wishFail(c, http.StatusNotFound, "Member not found")
			return
		}
		log.Printf("ERROR RemoveWishListMember: list %d, member %d: %v", list_id, member_id, err)
		wishFail(c, http.StatusInternalServerError, "Failed to remove member")
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Member removed",
		Data:    gin.H{"list_deleted": deleted},
	})
}

// GetItems returns all items in a shared list
func (h *WishListHandler) GetItems(c *gin.Context) {
	list_id, ok := h.memberListID(c)
	if !ok {
		return
	}
	items, err := db.GetWishListItems(h.DB, list_id)
	if err != nil {
		log.Printf("ERROR GetWishListItems: list %d: %v", list_id, err)
		wishFail(c, http.StatusInternalServerError, "Failed to load wishes")
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: items})
}

// CreateItem adds an item to a shared list
func (h *WishListHandler) CreateItem(c *gin.Context) {
	list_id, ok := h.memberListID(c)
	if !ok {
		return
	}
	req, ok := bindWishItem(c)
	if !ok {
		return
	}
	user_id := c.GetInt("userID")
	item, err := db.CreateWishListItem(h.DB, list_id, user_id, req.Name, req.Why, req.Link, req.Cost)
	if err != nil {
		log.Printf("ERROR CreateWishListItem: list %d, user %d: %v", list_id, user_id, err)
		wishFail(c, http.StatusInternalServerError, "Failed to add wish")
		return
	}
	c.JSON(http.StatusCreated, models.APIResponse{Success: true, Data: item})
}

// UpdateItem edits an item in a shared list
func (h *WishListHandler) UpdateItem(c *gin.Context) {
	item_id, ok := paramID(c, "itemId", "wish")
	if !ok {
		return
	}
	list_id, ok := h.memberListID(c)
	if !ok {
		return
	}
	req, ok := bindWishItem(c)
	if !ok {
		return
	}
	item, err := db.UpdateWishListItem(h.DB, list_id, item_id, req.Name, req.Why, req.Link, req.Cost)
	if err != nil {
		if err == db.ErrNotFound {
			wishFail(c, http.StatusNotFound, "Wish not found")
			return
		}
		log.Printf("ERROR UpdateWishListItem: list %d, wish %d: %v", list_id, item_id, err)
		wishFail(c, http.StatusInternalServerError, "Failed to update wish")
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: item})
}

// DeleteItem removes an item from a shared list
func (h *WishListHandler) DeleteItem(c *gin.Context) {
	item_id, ok := paramID(c, "itemId", "wish")
	if !ok {
		return
	}
	list_id, ok := h.memberListID(c)
	if !ok {
		return
	}
	if err := db.DeleteWishListItem(h.DB, list_id, item_id); err != nil {
		if err == db.ErrNotFound {
			wishFail(c, http.StatusNotFound, "Wish not found")
			return
		}
		log.Printf("ERROR DeleteWishListItem: list %d, wish %d: %v", list_id, item_id, err)
		wishFail(c, http.StatusInternalServerError, "Failed to delete wish")
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Message: "Wish deleted"})
}

func (h *WishListHandler) respondList(c *gin.Context, list_id int) {
	list, err := db.GetWishList(h.DB, list_id)
	if err != nil {
		if err == db.ErrNotFound {
			wishFail(c, http.StatusNotFound, "Wish list not found")
			return
		}
		log.Printf("ERROR GetWishList: list %d: %v", list_id, err)
		wishFail(c, http.StatusInternalServerError, "Failed to load wish list")
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: list})
}
