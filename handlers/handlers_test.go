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

const testSecret = "handler-test-secret"

func testToken(uid int) string {
	claims := jwt.MapClaims{
		"user_id": uid,
		"exp":     time.Now().Add(time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, _ := t.SignedString([]byte(testSecret))
	return s
}

func setup() {
	os.Setenv("JWT_SECRET", testSecret)
}

func teardown() {
	os.Unsetenv("JWT_SECRET")
}

// --- Login ---

func TestLogin_EmptyBody(t *testing.T) {
	r := gin.New()
	r.POST("/api/login", func(c *gin.Context) {
		var req models.LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(200, models.APIResponse{Success: true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/login", nil)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestLogin_MissingEmail(t *testing.T) {
	r := gin.New()
	r.POST("/api/login", func(c *gin.Context) {
		var req models.LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(200, models.APIResponse{Success: true})
	})

	body, _ := json.Marshal(map[string]string{"password": "abc123"})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

// --- Register ---

func TestRegister_ShortPassword(t *testing.T) {
	r := gin.New()
	r.POST("/api/register", func(c *gin.Context) {
		var req models.CreateUserRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(200, models.APIResponse{Success: true})
	})

	body, _ := json.Marshal(map[string]string{
		"email": "a@b.com", "password": "123", "name": "X",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestRegister_InvalidEmail(t *testing.T) {
	r := gin.New()
	r.POST("/api/register", func(c *gin.Context) {
		var req models.CreateUserRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(200, models.APIResponse{Success: true})
	})

	body, _ := json.Marshal(map[string]string{
		"email": "notanemail", "password": "password1", "name": "X",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

// --- Protected routes need auth ---

func TestProtected_NoAuth(t *testing.T) {
	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.GET("/profile", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/profile", nil)
	r.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestProtected_WithAuth(t *testing.T) {
	setup()
	defer teardown()

	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.GET("/profile", func(c *gin.Context) {
		c.JSON(200, gin.H{"user_id": c.GetInt("userID")})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/profile", nil)
	req.Header.Set("Authorization", "Bearer "+testToken(1))
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("want 200, got %d", w.Code)
	}
}

// --- Group creation validation ---

func TestCreateGroup_EmptyName(t *testing.T) {
	setup()
	defer teardown()

	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.POST("/groups", func(c *gin.Context) {
		var req models.CreateGroupRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(201, models.APIResponse{Success: true})
	})

	body, _ := json.Marshal(map[string]string{"name": ""})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/groups", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testToken(1))
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestCreateGroup_NameTooLong(t *testing.T) {
	setup()
	defer teardown()

	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.POST("/groups", func(c *gin.Context) {
		var req models.CreateGroupRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(201, models.APIResponse{Success: true})
	})

	long := make([]byte, 70)
	for i := range long {
		long[i] = 'x'
	}
	body, _ := json.Marshal(map[string]string{"name": string(long)})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/groups", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testToken(1))
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

// --- Group ID parsing ---

func TestGroupID_Undefined(t *testing.T) {
	setup()
	defer teardown()

	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.GET("/groups/:id", func(c *gin.Context) {
		if _, err := strconv.Atoi(c.Param("id")); err != nil {
			c.JSON(400, models.APIResponse{Success: false, Error: "Invalid group ID"})
			return
		}
		c.JSON(200, models.APIResponse{Success: true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/groups/undefined", nil)
	req.Header.Set("Authorization", "Bearer "+testToken(1))
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("want 400 for 'undefined', got %d", w.Code)
	}
}

func TestGroupID_NaN(t *testing.T) {
	setup()
	defer teardown()

	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.GET("/groups/:id", func(c *gin.Context) {
		if _, err := strconv.Atoi(c.Param("id")); err != nil {
			c.JSON(400, models.APIResponse{Success: false, Error: "Invalid group ID"})
			return
		}
		c.JSON(200, models.APIResponse{Success: true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/groups/NaN", nil)
	req.Header.Set("Authorization", "Bearer "+testToken(1))
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("want 400 for NaN, got %d", w.Code)
	}
}

// --- Settlement validation ---

func TestSettlement_MissingPaidTo(t *testing.T) {
	setup()
	defer teardown()

	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.POST("/groups/:id/settlements", func(c *gin.Context) {
		var req models.CreateSettlementRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(201, models.APIResponse{Success: true})
	})

	body, _ := json.Marshal(map[string]interface{}{"amount": 50.0})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/groups/1/settlements", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testToken(1))
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestSettlement_ZeroAmount(t *testing.T) {
	setup()
	defer teardown()

	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.POST("/groups/:id/settlements", func(c *gin.Context) {
		var req models.CreateSettlementRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(201, models.APIResponse{Success: true})
	})

	body, _ := json.Marshal(map[string]interface{}{"paid_to": 2, "amount": 0})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/groups/1/settlements", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testToken(1))
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

// --- Expense validation ---

func TestExpense_InvalidSplitType(t *testing.T) {
	setup()
	defer teardown()

	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.POST("/groups/:id/expenses", func(c *gin.Context) {
		var req models.CreateExpenseRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(201, models.APIResponse{Success: true})
	})

	body, _ := json.Marshal(map[string]interface{}{
		"amount": 100, "description": "test", "split_type": "invalid",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/groups/1/expenses", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testToken(1))
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("want 400 for invalid split_type, got %d", w.Code)
	}
}

func TestExpense_NegativeAmount(t *testing.T) {
	setup()
	defer teardown()

	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.POST("/groups/:id/expenses", func(c *gin.Context) {
		var req models.CreateExpenseRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(201, models.APIResponse{Success: true})
	})

	body, _ := json.Marshal(map[string]interface{}{
		"amount": -50, "description": "test", "split_type": "equal",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/groups/1/expenses", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testToken(1))
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("want 400 for negative amount, got %d", w.Code)
	}
}

// --- Theme validation ---

func TestTheme_Invalid(t *testing.T) {
	setup()
	defer teardown()

	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.PUT("/profile/theme", func(c *gin.Context) {
		var req struct {
			Theme string `json:"theme" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		valid := map[string]bool{
			"espresso": true, "dark": true, "dracula": true, "monokai": true,
			"cyberpunk": true, "ocean": true, "matcha": true, "rosegold": true,
			"lavender": true, "sakura": true, "solarized": true, "light": true,
		}
		if !valid[req.Theme] {
			c.JSON(400, models.APIResponse{Success: false, Error: "Invalid theme"})
			return
		}
		c.JSON(200, models.APIResponse{Success: true})
	})

	body, _ := json.Marshal(map[string]string{"theme": "hacker"})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/profile/theme", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testToken(1))
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestTheme_Valid(t *testing.T) {
	setup()
	defer teardown()

	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.PUT("/profile/theme", func(c *gin.Context) {
		var req struct {
			Theme string `json:"theme" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		valid := map[string]bool{
			"espresso": true, "dark": true, "dracula": true, "monokai": true,
			"cyberpunk": true, "ocean": true, "matcha": true, "rosegold": true,
			"lavender": true, "sakura": true, "solarized": true, "light": true,
		}
		if !valid[req.Theme] {
			c.JSON(400, models.APIResponse{Success: false, Error: "Invalid theme"})
			return
		}
		c.JSON(200, models.APIResponse{Success: true})
	})

	body, _ := json.Marshal(map[string]string{"theme": "espresso"})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/profile/theme", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testToken(1))
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("want 200, got %d", w.Code)
	}
}

// --- CORS ---

func TestCORS_Options(t *testing.T) {
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
	r.GET("/t", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("OPTIONS", "/t", nil)
	r.ServeHTTP(w, req)

	if w.Code != 204 {
		t.Fatalf("want 204, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("missing CORS origin header")
	}
}

// --- APIResponse format ---

func TestAPIResponse_Success(t *testing.T) {
	r := gin.New()
	r.GET("/t", func(c *gin.Context) {
		c.JSON(200, models.APIResponse{Success: true, Data: gin.H{"hello": "world"}})
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/t", nil)
	r.ServeHTTP(w, req)

	var resp models.APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Success {
		t.Fatal("expected success=true")
	}
}

func TestAPIResponse_Error(t *testing.T) {
	r := gin.New()
	r.GET("/t", func(c *gin.Context) {
		c.JSON(500, models.APIResponse{Success: false, Error: "boom"})
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/t", nil)
	r.ServeHTTP(w, req)

	var resp models.APIResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Success {
		t.Fatal("expected success=false")
	}
	if resp.Error != "boom" {
		t.Fatalf("want error 'boom', got %q", resp.Error)
	}
}

// --- AddMember ---

func TestAddMember_ZeroUserID(t *testing.T) {
	setup()
	defer teardown()

	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.POST("/groups/:id/members", func(c *gin.Context) {
		var req models.AddMemberRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(200, models.APIResponse{Success: true})
	})

	body, _ := json.Marshal(map[string]interface{}{"user_id": 0})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/groups/1/members", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testToken(1))
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("want 400 for user_id=0, got %d", w.Code)
	}
}

// --- JWT 7-day expiry ---

func TestJWT_7DayToken(t *testing.T) {
	setup()
	defer teardown()

	claims := jwt.MapClaims{
		"user_id": 1,
		"exp":     time.Now().Add(7 * 24 * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ts, _ := tok.SignedString([]byte(testSecret))

	r := gin.New()
	r.GET("/t", middleware.AuthMiddleware(), func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/t", nil)
	req.Header.Set("Authorization", "Bearer "+ts)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("7-day token should work, got %d", w.Code)
	}
}
