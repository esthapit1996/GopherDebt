package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"gopherdebt/db"
	"gopherdebt/models"
)

// GoCrumbs pages: named budgets (e.g. one per month), up to db.MaxGoCrumbsPages per user and
// work type. A page's work type is fixed once it is created. Another user's page is a 404.

// bindGoCrumbsPage parses and validates a page payload, writing a 400 on failure. Like a profile,
// a page must be calculable, so every saved page can be shown again.
func bindGoCrumbsPage(c *gin.Context) (*models.GoCrumbsPageRequest, bool) {
	var req models.GoCrumbsPageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		wishFail(c, http.StatusBadRequest, "Invalid request: "+err.Error())
		return nil, false
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		wishFail(c, http.StatusBadRequest, "Page name is required")
		return nil, false
	}
	if !cleanGoCrumbs(c, &req.GoCrumbsRequest) {
		return nil, false
	}
	if _, ok := calculateGoCrumbs(c, &req.GoCrumbsRequest); !ok {
		return nil, false
	}
	return &req, true
}

// goCrumbsPageFail writes the response for a failed page operation
func goCrumbsPageFail(c *gin.Context, op string, page_id int, err error) {
	switch err {
	case db.ErrNotFound:
		wishFail(c, http.StatusNotFound, "Page not found")
	case db.ErrDuplicate:
		wishFail(c, http.StatusConflict, "You already have a page with this name")
	case db.ErrGoCrumbsPageLimit:
		wishFail(c, http.StatusConflict, fmt.Sprintf("You can have at most %d pages per work type", db.MaxGoCrumbsPages))
	case db.ErrGoCrumbsWorkTypeLocked:
		wishFail(c, http.StatusBadRequest, "A page's work type can't be changed")
	default:
		log.Printf("ERROR %s: user %d, page %d: %v", op, c.GetInt("userID"), page_id, err)
		wishFail(c, http.StatusInternalServerError, "Failed to "+strings.ToLower(op[:1])+op[1:]+" the page")
	}
}

// ListGoCrumbsPages returns the user's pages (both work types), oldest first
func (h *GoCrumbsHandler) ListGoCrumbsPages(c *gin.Context) {
	pages, err := db.ListGoCrumbsPages(h.DB, c.GetInt("userID"))
	if err != nil {
		log.Printf("ERROR ListGoCrumbsPages: user %d: %v", c.GetInt("userID"), err)
		wishFail(c, http.StatusInternalServerError, "Failed to load your pages")
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: pages})
}

// GetGoCrumbsPage returns one full page
func (h *GoCrumbsHandler) GetGoCrumbsPage(c *gin.Context) {
	page_id, ok := paramID(c, "id", "page")
	if !ok {
		return
	}
	page, err := db.GetGoCrumbsPage(h.DB, c.GetInt("userID"), page_id)
	if err != nil {
		goCrumbsPageFail(c, "Load", page_id, err)
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: page})
}

// CreateGoCrumbsPage adds a page (409 at the limit or for a name already used in that work type)
func (h *GoCrumbsHandler) CreateGoCrumbsPage(c *gin.Context) {
	req, ok := bindGoCrumbsPage(c)
	if !ok {
		return
	}
	page, err := db.CreateGoCrumbsPage(h.DB, c.GetInt("userID"), req)
	if err != nil {
		goCrumbsPageFail(c, "Create", 0, err)
		return
	}
	c.JSON(http.StatusCreated, models.APIResponse{Success: true, Data: page})
}

// UpdateGoCrumbsPage saves (and renames) a page; its work type must stay the same
func (h *GoCrumbsHandler) UpdateGoCrumbsPage(c *gin.Context) {
	page_id, ok := paramID(c, "id", "page")
	if !ok {
		return
	}
	req, ok := bindGoCrumbsPage(c)
	if !ok {
		return
	}
	page, err := db.UpdateGoCrumbsPage(h.DB, c.GetInt("userID"), page_id, req)
	if err != nil {
		goCrumbsPageFail(c, "Save", page_id, err)
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: page})
}

// DeleteGoCrumbsPage deletes a page and its lines
func (h *GoCrumbsHandler) DeleteGoCrumbsPage(c *gin.Context) {
	page_id, ok := paramID(c, "id", "page")
	if !ok {
		return
	}
	if err := db.DeleteGoCrumbsPage(h.DB, c.GetInt("userID"), page_id); err != nil {
		goCrumbsPageFail(c, "Delete", page_id, err)
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true})
}
