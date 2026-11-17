package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"gopherdebt/db"
	"gopherdebt/models"
)

type WishHandler struct {
	DB *sql.DB
}

func NewWishHandler(database *sql.DB) *WishHandler {
	return &WishHandler{DB: database}
}

// validWishLink accepts an empty link or an absolute http(s) URL.
// Anything else (e.g. javascript:) is rejected since the frontend renders it as an href.
func validWishLink(link string) bool {
	if link == "" {
		return true
	}
	u, err := url.Parse(link)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// bindWishItem parses and validates a wish item payload, writing a 400 on failure
func bindWishItem(c *gin.Context) (*models.WishItemRequest, bool) {
	var req models.WishItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Invalid request: " + err.Error(),
		})
		return nil, false
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Link = strings.TrimSpace(req.Link)
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Name is required",
		})
		return nil, false
	}
	if !validWishLink(req.Link) {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Link must be an http(s) URL",
		})
		return nil, false
	}
	return &req, true
}

// GetWishItems returns all wishlist items for the authenticated user
func (h *WishHandler) GetWishItems(c *gin.Context) {
	userID := c.GetInt("userID")

	items, err := db.GetWishItems(h.DB, userID)
	if err != nil {
		log.Printf("ERROR GetWishItems: user %d: %v", userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to load wishlist",
		})
		return
	}

	if items == nil {
		items = []models.WishItem{}
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    items,
	})
}

// CreateWishItem adds an item to the wishlist
func (h *WishHandler) CreateWishItem(c *gin.Context) {
	userID := c.GetInt("userID")

	req, ok := bindWishItem(c)
	if !ok {
		return
	}

	item, err := db.CreateWishItem(h.DB, userID, req.Name, req.Link, req.Cost)
	if err != nil {
		log.Printf("ERROR CreateWishItem: user %d: %v", userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to add wish",
		})
		return
	}

	c.JSON(http.StatusCreated, models.APIResponse{
		Success: true,
		Data:    item,
	})
}

// UpdateWishItem edits a wishlist item owned by the authenticated user
func (h *WishHandler) UpdateWishItem(c *gin.Context) {
	userID := c.GetInt("userID")
	itemID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Invalid wish ID",
		})
		return
	}

	req, ok := bindWishItem(c)
	if !ok {
		return
	}

	item, err := db.UpdateWishItem(h.DB, itemID, userID, req.Name, req.Link, req.Cost)
	if err != nil {
		if err == db.ErrNotFound {
			c.JSON(http.StatusNotFound, models.APIResponse{
				Success: false,
				Error:   "Wish not found",
			})
			return
		}
		log.Printf("ERROR UpdateWishItem: user %d, wish %d: %v", userID, itemID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to update wish",
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    item,
	})
}

// DeleteWishItem removes a wishlist item
func (h *WishHandler) DeleteWishItem(c *gin.Context) {
	userID := c.GetInt("userID")
	itemID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Invalid wish ID",
		})
		return
	}

	if err := db.DeleteWishItem(h.DB, itemID, userID); err != nil {
		if err == db.ErrNotFound {
			c.JSON(http.StatusNotFound, models.APIResponse{
				Success: false,
				Error:   "Wish not found",
			})
			return
		}
		log.Printf("ERROR DeleteWishItem: user %d, wish %d: %v", userID, itemID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to delete wish",
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Wish deleted",
	})
}
