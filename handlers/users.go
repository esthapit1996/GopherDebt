package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"gopherdebt/db"
	"gopherdebt/models"
)

type UserHandler struct {
	DB *sql.DB
}

func NewUserHandler(database *sql.DB) *UserHandler {
	return &UserHandler{DB: database}
}

func (h *UserHandler) Register(c *gin.Context) {
	var req models.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	// Check if email is blacklisted first
	blacklisted, err := db.IsEmailBlacklisted(h.DB, req.Email)
	if err != nil {
		log.Printf("ERROR Register: IsEmailBlacklisted failed for %s: %v", req.Email, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to check access"})
		return
	}
	if blacklisted {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "This email has been blocked from registration"})
		return
	}

	// Check if email is whitelisted
	whitelisted, err := db.IsEmailWhitelisted(h.DB, req.Email)
	if err != nil {
		log.Printf("ERROR Register: IsEmailWhitelisted failed for %s: %v", req.Email, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to check access"})
		return
	}
	if !whitelisted {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "You are not worthy!"})
		return
	}

	_, err = db.GetUserByEmail(h.DB, req.Email)
	if err == nil {
		c.JSON(http.StatusConflict, models.APIResponse{Success: false, Error: "User with this email already exists"})
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("ERROR Register: bcrypt failed: %v", err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to process password"})
		return
	}

	user, err := db.CreateUser(h.DB, req.Email, string(hashedPassword), req.Name)
	if err != nil {
		log.Printf("ERROR Register: CreateUser failed for %s: %v", req.Email, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to create user"})
		return
	}

	c.JSON(http.StatusCreated, models.APIResponse{Success: true, Message: "User created successfully", Data: user})
}

func (h *UserHandler) Login(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	user, err := db.GetUserByEmail(h.DB, req.Email)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.APIResponse{Success: false, Error: "Invalid email or password"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, models.APIResponse{Success: false, Error: "Invalid email or password"})
		return
	}

	token, err := generateJWT(user.ID)
	if err != nil {
		log.Printf("ERROR Login: generateJWT failed for user %d: %v", user.ID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: models.LoginResponse{Token: token, User: *user}})
}

func (h *UserHandler) GetProfile(c *gin.Context) {
	userID := c.GetInt("userID")
	user, err := db.GetUserByID(h.DB, userID)
	if err == db.ErrNotFound {
		c.JSON(http.StatusNotFound, models.APIResponse{Success: false, Error: "User not found"})
		return
	}
	if err != nil {
		log.Printf("ERROR GetProfile: GetUserByID failed for user %d: %v", userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to load profile"})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: user})
}

func (h *UserHandler) GetAllUsers(c *gin.Context) {
	users, err := db.GetAllUsers(h.DB)
	if err != nil {
		log.Printf("ERROR GetAllUsers: %v", err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to fetch users"})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: users})
}

func (h *UserHandler) GetDebtOverview(c *gin.Context) {
	userID := c.GetInt("userID")
	overview, err := db.GetDebtOverview(h.DB, userID)
	if err != nil {
		log.Printf("ERROR GetDebtOverview: user %d: %v", userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to fetch debt overview"})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: overview})
}

func (h *UserHandler) GetDebtDetails(c *gin.Context) {
	userID := c.GetInt("userID")
	otherUserID, err := strconv.Atoi(c.Param("userId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid user ID"})
		return
	}

	details, err := db.GetDebtDetails(h.DB, userID, otherUserID)
	if err != nil {
		log.Printf("ERROR GetDebtDetails: user %d, other %d: %v", userID, otherUserID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to fetch debt details"})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: details})
}

func (h *UserHandler) GetPaymentHistory(c *gin.Context) {
	userID := c.GetInt("userID")
	history, err := db.GetPaymentHistory(h.DB, userID)
	if err != nil {
		log.Printf("ERROR GetPaymentHistory: user %d: %v", userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to fetch payment history"})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: history})
}

func (h *UserHandler) ClearPaymentHistory(c *gin.Context) {
	userID := c.GetInt("userID")
	err := db.ClearPaymentHistory(h.DB, userID)
	if err != nil {
		log.Printf("ERROR ClearPaymentHistory: user %d: %v", userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to clear payment history"})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: "Payment history cleared"})
}

func (h *UserHandler) UpdateTheme(c *gin.Context) {
	userID := c.GetInt("userID")

	var req struct {
		Theme string `json:"theme" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	// Validate theme
	validThemes := map[string]bool{
		"espresso": true, "darkknight": true, "dracula": true, "monokai": true,
		"cyberpunk": true, "ocean": true, "matcha": true, "rosegold": true, "purplehaze": true,
		"lavender": true, "sakura": true, "cottoncandy": true, "solarized": true, "flashbang": true,
	}
	if !validThemes[req.Theme] {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid theme"})
		return
	}

	err := db.UpdateUserTheme(h.DB, userID, req.Theme)
	if err != nil {
		log.Printf("ERROR UpdateTheme: user %d, theme %s: %v", userID, req.Theme, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to update theme"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Message: "Theme updated"})
}

func (h *UserHandler) UpdateAvatar(c *gin.Context) {
	userID := c.GetInt("userID")

	var req struct {
		Avatar string `json:"avatar" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	// Validate avatar against known filenames
	validAvatars := map[string]bool{
		"camel": true, "cat": true, "dog": true, "duck": true,
		"elephant": true, "flower": true, "gopher": true,
		"mafia_1": true, "mafia_2": true, "monkey": true,
		"mouse": true, "rhino": true, "": true,
	}
	if !validAvatars[req.Avatar] {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid avatar"})
		return
	}

	err := db.UpdateUserAvatar(h.DB, userID, req.Avatar)
	if err != nil {
		log.Printf("ERROR UpdateAvatar: user %d, avatar %s: %v", userID, req.Avatar, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to update avatar"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Message: "Avatar updated"})
}

func (h *UserHandler) UpdateLanguage(c *gin.Context) {
	userID := c.GetInt("userID")

	var req struct {
		Language string `json:"language" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	// Validate language
	validLanguages := map[string]bool{
		"en": true, "it": true,
	}
	if !validLanguages[req.Language] {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid language"})
		return
	}

	err := db.UpdateUserLanguage(h.DB, userID, req.Language)
	if err != nil {
		log.Printf("ERROR UpdateLanguage: user %d, language %s: %v", userID, req.Language, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to update language"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Message: "Language updated"})
}

func (h *UserHandler) ChangePassword(c *gin.Context) {
	userID := c.GetInt("userID")

	var req models.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	if req.NewPassword != req.ConfirmPassword {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "New passwords do not match"})
		return
	}

	if req.OldPassword == req.NewPassword {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "New password must be different from old password"})
		return
	}

	// Get current password hash
	currentHash, err := db.GetUserPasswordHash(h.DB, userID)
	if err != nil {
		log.Printf("ERROR ChangePassword: GetUserPasswordHash failed for user %d: %v", userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to verify current password"})
		return
	}

	// Verify old password
	if err := bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(req.OldPassword)); err != nil {
		c.JSON(http.StatusUnauthorized, models.APIResponse{Success: false, Error: "Current password is incorrect"})
		return
	}

	// Hash new password
	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("ERROR ChangePassword: bcrypt failed for user %d: %v", userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to process new password"})
		return
	}

	// Update password
	if err := db.UpdateUserPassword(h.DB, userID, string(newHash)); err != nil {
		log.Printf("ERROR ChangePassword: UpdateUserPassword failed for user %d: %v", userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to update password"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Message: "Password changed successfully"})
}

func generateJWT(userID int) (string, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "your-secret-key-change-in-production"
	}

	claims := jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(time.Hour * 24 * 7).Unix(),
		"iat":     time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func (h *UserHandler) DeleteUser(c *gin.Context) {
	// Get the requesting user
	requesterID := c.GetInt("userID")
	requester, err := db.GetUserByID(h.DB, requesterID)
	if err != nil {
		log.Printf("ERROR DeleteUser: GetUserByID failed for requester %d: %v", requesterID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to verify requester"})
		return
	}

	// Only founder can delete users
	if requester.Email != db.FounderEmail {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "Only the founder can delete users"})
		return
	}

	// Get user ID to delete
	userIDStr := c.Param("id")
	userID, err := strconv.Atoi(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid user ID"})
		return
	}

	// Get the user to be deleted
	userToDelete, err := db.GetUserByID(h.DB, userID)
	if err != nil {
		if err == db.ErrNotFound {
			c.JSON(http.StatusNotFound, models.APIResponse{Success: false, Error: "User not found"})
			return
		}
		log.Printf("ERROR DeleteUser: GetUserByID failed for target %d: %v", userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to fetch user"})
		return
	}

	// Cannot delete the founder
	if userToDelete.Email == db.FounderEmail {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "Cannot delete the founder account"})
		return
	}

	// Delete the user
	if err := db.DeleteUser(h.DB, userID); err != nil {
		log.Printf("ERROR DeleteUser: DeleteUser failed for %d: %v", userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to delete user"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: "User deleted successfully"})
}
