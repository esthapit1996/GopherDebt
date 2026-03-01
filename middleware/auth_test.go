package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func makeToken(userID int, secret string, expiry time.Duration) string {
	claims := jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(expiry).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, _ := token.SignedString([]byte(secret))
	return s
}

func TestNoHeader(t *testing.T) {
	r := gin.New()
	r.GET("/t", AuthMiddleware(), func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/t", nil)
	r.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestBadFormat(t *testing.T) {
	r := gin.New()
	r.GET("/t", AuthMiddleware(), func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/t", nil)
	req.Header.Set("Authorization", "Token abc")
	r.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestExpiredToken(t *testing.T) {
	secret := "test-secret"
	os.Setenv("JWT_SECRET", secret)
	defer os.Unsetenv("JWT_SECRET")

	tok := makeToken(1, secret, -time.Hour)

	r := gin.New()
	r.GET("/t", AuthMiddleware(), func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/t", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Fatalf("want 401 for expired, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "Invalid or expired token" {
		t.Fatalf("unexpected error: %v", resp["error"])
	}
}

func TestValidToken(t *testing.T) {
	secret := "test-secret"
	os.Setenv("JWT_SECRET", secret)
	defer os.Unsetenv("JWT_SECRET")

	tok := makeToken(42, secret, time.Hour)

	r := gin.New()
	r.GET("/t", AuthMiddleware(), func(c *gin.Context) {
		c.JSON(200, gin.H{"user_id": c.GetInt("userID")})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/t", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if int(resp["user_id"].(float64)) != 42 {
		t.Fatalf("want user_id=42, got %v", resp["user_id"])
	}
}

func TestWrongSecret(t *testing.T) {
	os.Setenv("JWT_SECRET", "real-secret")
	defer os.Unsetenv("JWT_SECRET")

	tok := makeToken(1, "wrong-secret", time.Hour)

	r := gin.New()
	r.GET("/t", AuthMiddleware(), func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/t", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Fatalf("want 401 for wrong secret, got %d", w.Code)
	}
}

func TestFallbackSecret(t *testing.T) {
	os.Unsetenv("JWT_SECRET")

	tok := makeToken(7, "your-secret-key-change-in-production", time.Hour)

	r := gin.New()
	r.GET("/t", AuthMiddleware(), func(c *gin.Context) {
		c.JSON(200, gin.H{"user_id": c.GetInt("userID")})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/t", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("want 200 with fallback secret, got %d", w.Code)
	}
}

func TestMissingUserIDClaim(t *testing.T) {
	secret := "test-secret"
	os.Setenv("JWT_SECRET", secret)
	defer os.Unsetenv("JWT_SECRET")

	claims := jwt.MapClaims{
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ts, _ := token.SignedString([]byte(secret))

	r := gin.New()
	r.GET("/t", AuthMiddleware(), func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/t", nil)
	req.Header.Set("Authorization", "Bearer "+ts)
	r.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Fatalf("want 401 for missing user_id, got %d", w.Code)
	}
}
