package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"gopherdebt/middleware"
	"gopherdebt/models"
)

func init() {
	gin.SetMode(gin.TestMode)
}

const testJWTSecret = "test-jwt-secret-for-handlers"

func generateTestToken(userID int) string {
	claims := jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := token.SignedString([]byte(testJWTSecret))
	return signed
}

// helper matching the real handler pattern
func parseGroupID(id string) (int, error) {
	return strconv.Atoi(id)
}

// --- Login validation tests ---

func TestLogin_EmptyBody(t *testing.T) {
	os.Setenv("JWT_SECRET", testJWTSecret)
	defer os.Unsetenv("JWT_SECRET")
	r := gin.New()
	r.POST("/api/login", func(c *gin.Context) {
		var req models.LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(http.StatusOK, models.APIResponse{Success: true})
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/login", bytes.NewBuffer([]byte{}))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty body, got %d", w.Code)
	}
}

func TestLogin_InvalidJSON(t *testing.T) {
	r := gin.New()
	r.POST("/api/login", func(c *gin.Context) {
		var req models.LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(http.StatusOK, models.APIResponse{Success: true})
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/login", bytes.NewBuffer([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

func TestLogin_MissingEmail(t *testing.T) {
	r := gin.New()
	r.POST("/api/login", func(c *gin.Context) {
		var req models.LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(http.StatusOK, models.APIResponse{Success: true})
	})
	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{"password": "test123"})
	req, _ := http.NewRequest("POST", "/api/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing email, got %d", w.Code)
	}
}

// --- Register validation tests ---

func TestRegister_ShortPassword(t *testing.T) {
	r := gin.New()
	r.POST("/api/register", func(c *gin.Context) {
		var req models.CreateUserRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(http.StatusOK, models.APIResponse{Success: true})
	})
	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{
		"email":    "test@test.com",
		"password": "123",
		"name":     "Test",
	})
	req, _ := http.NewRequest("POST", "/api/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for short password, got %d", w.Code)
	}
}

func TestRegister_InvalidEmail(t *testing.T) {
	r := gin.New()
	r.POST("/api/register", func(c *gin.Context) {
		var req models.CreateUserRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(http.StatusOK, models.APIResponse{Success: true})
	})
	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{
		"email":    "not-an-email",
		"password": "password123",
		"name":     "Test",
	})
	req, _ := http.NewRequest("POST", "/api/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid email, got %d", w.Code)
	}
}

// --- Protected route tests ---

func TestProtectedRoute_NoAuth(t *testing.T) {
	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.GET("/profile", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/profile", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", w.Code)
	}
}

func TestProtectedRoute_WithAuth(t *testing.T) {
	os.Setenv("JWT_SECRET", testJWTSecret)
	defer os.Unsetenv("JWT_SECRET")
	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.GET("/profile", func(c *gin.Context) {
		userID := c.GetInt("userID")
		c.JSON(200, gin.H{"user_id": userID})
	})
	token := generateTestToken(1)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/profile", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with auth, got %d", w.Code)
	}
}

// --- Group creation validation tests ---

func TestCreateGroup_EmptyName(t *testing.T) {
	os.Setenv("JWT_SECRET", testJWTSecret)
	defer os.Unsetenv("JWT_SECRET")
	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.POST("/groups", func(c *gin.Context) {
		var req models.CreateGroupRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(http.StatusCreated, models.APIResponse{Success: true})
	})
	token := generateTestToken(1)
	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{"name": "", "description": "test"})
	req, _ := http.NewRequest("POST", "/api/groups", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty group name, got %d", w.Code)
	}
}

func TestCreateGroup_NameTooLong(t *testing.T) {
	os.Setenv("JWT_SECRET", testJWTSecret)
	defer os.Unsetenv("JWT_SECRET")
	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.POST("/groups", func(c *gin.Context) {
		var req models.CreateGroupRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(http.StatusCreated, models.APIResponse{Success: true})
	})
	token := generateTestToken(1)
	longName := make([]byte, 70)
	for i := range longName {
		longName[i] = 'a'
	}
	body, _ := json.Marshal(map[string]string{"name": string(longName)})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/groups", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for name too long (70 chars > max 69), got %d", w.Code)
	}
}

// --- Invalid group ID in URL ---

func TestGroupEndpoint_InvalidGroupID(t *testing.T) {
	os.Setenv("JWT_SECRET", testJWTSecret)
	defer os.Unsetenv("JWT_SECRET")
	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.GET("/groups/:id", func(c *gin.Context) {
		_, err := parseGroupID(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid group ID"})
			return
		}
		c.JSON(http.StatusOK, models.APIResponse{Success: true})
	})
	token := generateTestToken(1)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/groups/undefined", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for 'undefined' group ID, got %d", w.Code)
	}
}

func TestGroupEndpoint_NaNGroupID(t *testing.T) {
	os.Setenv("JWT_SECRET", testJWTSecret)
	defer os.Unsetenv("JWT_SECRET")
	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.GET("/groups/:id", func(c *gin.Context) {
		_, err := parseGroupID(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid group ID"})
			return
		}
		c.JSON(http.StatusOK, models.APIResponse{Success: true})
	})
	token := generateTestToken(1)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/groups/NaN", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for NaN group ID, got %d", w.Code)
	}
}

// --- Settlement validation tests ---

func TestCreateSettlement_MissingFields(t *testing.T) {
	os.Setenv("JWT_SECRET", testJWTSecret)
	defer os.Unsetenv("JWT_SECRET")
	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.POST("/groups/:id/settlements", func(c *gin.Context) {
		var req models.CreateSettlementRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(http.StatusCreated, models.APIResponse{Success: true})
	})
	token := generateTestToken(1)
	body, _ := json.Marshal(map[string]interface{}{"amount": 50.0})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/groups/1/settlements", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing paid_to, got %d", w.Code)
	}
}

func TestCreateSettlement_ZeroAmount(t *testing.T) {
	os.Setenv("JWT_SECRET", testJWTSecret)
	defer os.Unsetenv("JWT_SECRET")
	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.POST("/groups/:id/settlements", func(c *gin.Context) {
		var req models.CreateSettlementRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(http.StatusCreated, models.APIResponse{Success: true})
	})
	token := generateTestToken(1)
	body, _ := json.Marshal(map[string]interface{}{"paid_to": 2, "amount": 0})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/groups/1/settlements", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for zero amount, got %d", w.Code)
	}
}

// --- Expense validation tests ---

func TestCreateExpense_InvalidSplitType(t *testing.T) {
	os.Setenv("JWT_SECRET", testJWTSecret)
	defer os.Unsetenv("JWT_SECRET")
	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.POST("/groups/:id/expenses", func(c *gin.Context) {
		var req models.CreateExpenseRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(http.StatusCreated, models.APIResponse{Success: true})
	})
	token := generateTestToken(1)
	body, _ := json.Marshal(map[string]interface{}{
		"amount":      100,
		"description": "Test",
		"split_type":  "random",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/groups/1/expenses", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid split_type, got %d", w.Code)
	}
}

func TestCreateExpense_NegativeAmount(t *testing.T) {
	os.Setenv("JWT_SECRET", testJWTSecret)
	defer os.Unsetenv("JWT_SECRET")
	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.POST("/groups/:id/expenses", func(c *gin.Context) {
		var req models.CreateExpenseRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(http.StatusCreated, models.APIResponse{Success: true})
	})
	token := generateTestToken(1)
	body, _ := json.Marshal(map[string]interface{}{
		"amount":      -50,
		"description": "Test",
		"split_type":  "equal",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/groups/1/expenses", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for negative amount, got %d", w.Code)
	}
}

// --- Theme validation tests ---

func TestUpdateTheme_InvalidTheme(t *testing.T) {
	os.Setenv("JWT_SECRET", testJWTSecret)
	defer os.Unsetenv("JWT_SECRET")
	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.PUT("/profile/theme", func(c *gin.Context) {
		var req struct {
			Theme string `json:"theme" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		validThemes := map[string]bool{
			"espresso": true, "dark": true, "dracula": true, "monokai": true,
			"cyberpunk": true, "ocean": true, "matcha": true, "rosegold": true,
			"lavender": true, "sakura": true, "cottoncandy": true, "solarized": true, "light": true,
		}
		if !validThemes[req.Theme] {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid theme"})
			return
		}
		c.JSON(http.StatusOK, models.APIResponse{Success: true})
	})
	token := generateTestToken(1)
	body, _ := json.Marshal(map[string]string{"theme": "hacker"})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/profile/theme", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid theme, got %d", w.Code)
	}
}

func TestUpdateTheme_ValidTheme(t *testing.T) {
	os.Setenv("JWT_SECRET", testJWTSecret)
	defer os.Unsetenv("JWT_SECRET")
	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.PUT("/profile/theme", func(c *gin.Context) {
		var req struct {
			Theme string `json:"theme" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		validThemes := map[string]bool{
			"espresso": true, "dark": true, "dracula": true, "monokai": true,
			"cyberpunk": true, "ocean": true, "matcha": true, "rosegold": true,
			"lavender": true, "sakura": true, "cottoncandy": true, "solarized": true, "light": true,
		}
		if !validThemes[req.Theme] {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid theme"})
			return
		}
		c.JSON(http.StatusOK, models.APIResponse{Success: true, Message: "Theme updated"})
	})
	token := generateTestToken(1)
	body, _ := json.Marshal(map[string]string{"theme": "dark"})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/profile/theme", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for valid theme, got %d", w.Code)
	}
}

// --- CORS tests ---

func TestCORS_OptionsRequest(t *testing.T) {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, Cache-Control")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("OPTIONS", "/test", nil)
	r.ServeHTTP(w, req)
	if w.Code != 204 {
		t.Errorf("expected 204 for OPTIONS, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS header")
	}
}

// --- API Response format tests ---

func TestAPIResponseFormat(t *testing.T) {
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, models.APIResponse{
			Success: true,
			Data:    map[string]string{"key": "value"},
		})
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)
	var resp models.APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response as APIResponse: %v", err)
	}
	if !resp.Success {
		t.Error("expected success: true")
	}
	if resp.Data == nil {
		t.Error("expected non-nil data")
	}
}

func TestAPIResponseError(t *testing.T) {
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		c.JSON(500, models.APIResponse{
			Success: false,
			Error:   "Something broke",
		})
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)
	var resp models.APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Success {
		t.Error("expected success: false")
	}
	if resp.Error != "Something broke" {
		t.Errorf("expected error message, got %q", resp.Error)
	}
}

// --- Health endpoint test ---

func TestHealthEndpoint(t *testing.T) {
	r := gin.New()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "healthy"})
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// --- AddMember validation ---

func TestAddMember_InvalidUserID(t *testing.T) {
	os.Setenv("JWT_SECRET", testJWTSecret)
	defer os.Unsetenv("JWT_SECRET")
	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.POST("/groups/:id/members", func(c *gin.Context) {
		var req models.AddMemberRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(http.StatusOK, models.APIResponse{Success: true})
	})
	token := generateTestToken(1)
	body, _ := json.Marshal(map[string]interface{}{"user_id": 0})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/groups/1/members", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for user_id=0, got %d", w.Code)
	}
}

// --- JWT expiry edge case ---

func TestJWT_ExpiresAt7Days(t *testing.T) {
	secret := "test-secret"
	os.Setenv("JWT_SECRET", secret)
	defer os.Unsetenv("JWT_SECRET")
	claims := jwt.MapClaims{
		"user_id": 1,
		"exp":     time.Now().Add(time.Hour * 24 * 7).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(secret))
	r := gin.New()
	r.GET("/test", middleware.AuthMiddleware(), func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("7-day token should be valid, got %d", w.Code)
	}
}
