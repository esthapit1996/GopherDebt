package handlers

import (
	"database/sql"
	"net/http"
	"os"
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

// Whitelist of allowed emails for registration
var allowedEmails = map[string]bool{
	"evansthapit20@gmail.com":  true,
	"e.ivanishcheva@yandex.ru": true,
}

func (h *UserHandler) Register(c *gin.Context) {
	var req models.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
		return
	}

	// Check if email is whitelisted
	if !allowedEmails[req.Email] {
		c.JSON(http.StatusForbidden, models.APIResponse{Success: false, Error: "Sorry but the creator has deemed you unworthy! To request access, email evansthapit20@gmail.com"})
		return
	}

	_, err := db.GetUserByEmail(h.DB, req.Email)
	if err == nil {
		c.JSON(http.StatusConflict, models.APIResponse{Success: false, Error: "User with this email already exists"})
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to process password"})
		return
	}

	user, err := db.CreateUser(h.DB, req.Email, string(hashedPassword), req.Name)
	if err != nil {
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
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: models.LoginResponse{Token: token, User: *user}})
}

func (h *UserHandler) GetProfile(c *gin.Context) {
	userID := c.GetInt("userID")
	user, err := db.GetUserByID(h.DB, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.APIResponse{Success: false, Error: "User not found"})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: user})
}

func (h *UserHandler) GetAllUsers(c *gin.Context) {
	users, err := db.GetAllUsers(h.DB)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to fetch users"})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: users})
}

func (h *UserHandler) GetDebtOverview(c *gin.Context) {
	userID := c.GetInt("userID")
	overview, err := db.GetDebtOverview(h.DB, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to fetch debt overview"})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: overview})
}

func (h *UserHandler) GetPaymentHistory(c *gin.Context) {
	userID := c.GetInt("userID")
	history, err := db.GetPaymentHistory(h.DB, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to fetch payment history"})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: history})
}

func (h *UserHandler) ClearPaymentHistory(c *gin.Context) {
	userID := c.GetInt("userID")
	err := db.ClearPaymentHistory(h.DB, userID)
	if err != nil {
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
		"espresso": true, "dark": true, "dracula": true, "monokai": true,
		"cyberpunk": true, "ocean": true, "matcha": true, "rosegold": true,
		"lavender": true, "sakura": true, "solarized": true, "light": true,
	}
	if !validThemes[req.Theme] {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid theme"})
		return
	}

	err := db.UpdateUserTheme(h.DB, userID, req.Theme)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to update theme"})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{Success: true, Message: "Theme updated"})
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
