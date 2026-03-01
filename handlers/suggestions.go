package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"gopherdebt/db"
	"gopherdebt/models"
)

// Creator email - only this user can delete any suggestion
const CreatorEmail = "evansthapit20@gmail.com"

type SuggestionHandler struct {
	DB *sql.DB
}

func NewSuggestionHandler(database *sql.DB) *SuggestionHandler {
	return &SuggestionHandler{DB: database}
}

type CreateSuggestionRequest struct {
	Content string `json:"content" binding:"required,max=800"`
}

type VoteRequest struct {
	VoteType string `json:"vote_type" binding:"required,oneof=like dislike"`
}

// GetSuggestions returns all suggestions
func (h *SuggestionHandler) GetSuggestions(c *gin.Context) {
	userID := c.GetInt("userID")
	suggestions, err := db.GetAllSuggestions(h.DB, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to fetch suggestions"})
		return
	}

	// Get open count for limit info
	openCount, _ := db.GetOpenSuggestionCount(h.DB)

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data: gin.H{
			"suggestions": suggestions,
			"count":       openCount,
			"max":         db.MaxSuggestions,
		},
	})
}

// CreateSuggestion creates a new suggestion
func (h *SuggestionHandler) CreateSuggestion(c *gin.Context) {
	userID := c.GetInt("userID")

	// Check current open count
	openCount, err := db.GetOpenSuggestionCount(h.DB)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to check suggestion count"})
		return
	}

	if openCount >= db.MaxSuggestions {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Maximum open suggestions reached (20). Please wait for suggestions to be addressed."})
		return
	}

	var req CreateSuggestionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Content is required and must be max 800 characters"})
		return
	}

	suggestion, err := db.CreateSuggestion(h.DB, userID, req.Content)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to create suggestion"})
		return
	}

	c.JSON(http.StatusCreated, models.APIResponse{Success: true, Data: suggestion})
}

// DeleteSuggestion deletes a suggestion (only creator of suggestion or app creator can delete)
func (h *SuggestionHandler) DeleteSuggestion(c *gin.Context) {
	userID := c.GetInt("userID")
	suggestionID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid suggestion ID"})
		return
	}

	// Get the suggestion to check ownership
	suggestion, err := db.GetSuggestionByID(h.DB, suggestionID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.APIResponse{Success: false, Error: "Suggestion not found"})
		return
	}

	// Get current user's email
	user, err := db.GetUserByID(h.DB, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to verify user"})
		return
	}

	// Check if user is the creator of the suggestion OR the app creator
	if suggestion.UserID != userID && user.Email != CreatorEmail {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "Only the suggestion author or app creator can delete this"})
		return
	}

	if err := db.DeleteSuggestion(h.DB, suggestionID); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to delete suggestion"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: "Suggestion deleted"})
}

// VoteSuggestion adds or updates a vote on a suggestion
func (h *SuggestionHandler) VoteSuggestion(c *gin.Context) {
	userID := c.GetInt("userID")
	suggestionID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid suggestion ID"})
		return
	}

	var req VoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "vote_type must be 'like' or 'dislike'"})
		return
	}

	// Check suggestion exists
	_, err = db.GetSuggestionByID(h.DB, suggestionID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.APIResponse{Success: false, Error: "Suggestion not found"})
		return
	}

	if err := db.VoteSuggestion(h.DB, suggestionID, userID, req.VoteType); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to record vote"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: "Vote recorded"})
}

// RemoveVote removes a user's vote from a suggestion
func (h *SuggestionHandler) RemoveVote(c *gin.Context) {
	userID := c.GetInt("userID")
	suggestionID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid suggestion ID"})
		return
	}

	if err := db.RemoveVote(h.DB, suggestionID, userID); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to remove vote"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: "Vote removed"})
}

// GetVoters returns who voted on a suggestion (only for app creator)
func (h *SuggestionHandler) GetVoters(c *gin.Context) {
	userID := c.GetInt("userID")
	suggestionID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid suggestion ID"})
		return
	}

	// Check if user is the app creator
	user, err := db.GetUserByID(h.DB, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to verify user"})
		return
	}

	if user.Email != CreatorEmail {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "Only the app creator can view voters"})
		return
	}

	votes, err := db.GetSuggestionVotes(h.DB, suggestionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to fetch voters"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: votes})
}

type UpdateStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=open wip done"`
}

// UpdateSuggestionStatus updates the status of a suggestion (only for app creator)
func (h *SuggestionHandler) UpdateSuggestionStatus(c *gin.Context) {
	userID := c.GetInt("userID")
	suggestionID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid suggestion ID"})
		return
	}

	// Check if user is the app creator
	user, err := db.GetUserByID(h.DB, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to verify user"})
		return
	}

	if user.Email != CreatorEmail {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "Only the app creator can change suggestion status"})
		return
	}

	var req UpdateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Status must be 'open', 'wip', or 'done'"})
		return
	}

	if err := db.UpdateSuggestionStatus(h.DB, suggestionID, req.Status); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to update status"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: "Status updated"})
}
