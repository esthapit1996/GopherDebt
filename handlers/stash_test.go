package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"

	"gopherdebt/middleware"
	"gopherdebt/models"
)

func TestUpdateStash_InvalidID(t *testing.T) {
	os.Setenv("JWT_SECRET", testJWTSecret)
	defer os.Unsetenv("JWT_SECRET")
	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.PUT("/stash/:id", func(c *gin.Context) {
		if _, err := strconv.Atoi(c.Param("id")); err != nil {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "Invalid expense ID"})
			return
		}
		c.JSON(http.StatusOK, models.APIResponse{Success: true})
	})

	token := generateTestToken(1)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/stash/abc", bytes.NewBuffer([]byte(`{"amount": 10.0}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid stash id, got %d", w.Code)
	}
}

func TestUpdateStash_MissingAmount(t *testing.T) {
	os.Setenv("JWT_SECRET", testJWTSecret)
	defer os.Unsetenv("JWT_SECRET")
	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.PUT("/stash/:id", func(c *gin.Context) {
		var req models.CreateStashExpenseRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(http.StatusOK, models.APIResponse{Success: true})
	})

	token := generateTestToken(1)
	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{"description": "Lunch"})
	req, _ := http.NewRequest("PUT", "/api/stash/1", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing amount, got %d", w.Code)
	}
}

func TestUpdateStash_InvalidAmount(t *testing.T) {
	os.Setenv("JWT_SECRET", testJWTSecret)
	defer os.Unsetenv("JWT_SECRET")
	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.PUT("/stash/:id", func(c *gin.Context) {
		var req models.CreateStashExpenseRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(http.StatusOK, models.APIResponse{Success: true})
	})

	token := generateTestToken(1)
	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]float64{"amount": 0})
	req, _ := http.NewRequest("PUT", "/api/stash/1", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-positive amount, got %d", w.Code)
	}
}

func TestUpdateStash_ValidBody(t *testing.T) {
	os.Setenv("JWT_SECRET", testJWTSecret)
	defer os.Unsetenv("JWT_SECRET")
	r := gin.New()
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.PUT("/stash/:id", func(c *gin.Context) {
		var req models.CreateStashExpenseRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: err.Error()})
			return
		}
		c.JSON(http.StatusOK, models.APIResponse{Success: true})
	})

	token := generateTestToken(1)
	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]interface{}{"amount": 12.5, "description": "Coffee"})
	req, _ := http.NewRequest("PUT", "/api/stash/1", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for valid body, got %d", w.Code)
	}
}
