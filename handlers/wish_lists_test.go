package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"gopherdebt/models"
)

// All of these fail on path-param parsing, before any DB access, so a nil DB is safe.
func TestWishListHandler_InvalidIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewWishListHandler(nil)
	r := gin.New()
	r.GET("/wish-lists/:id", h.GetWishList)
	r.PUT("/wish-lists/:id", h.RenameWishList)
	r.POST("/wish-lists/:id/members", h.AddMember)
	r.DELETE("/wish-lists/:id/members/:userId", h.RemoveMember)
	r.GET("/wish-lists/:id/items", h.GetItems)
	r.POST("/wish-lists/:id/items", h.CreateItem)
	r.PUT("/wish-lists/:id/items/:itemId", h.UpdateItem)
	r.DELETE("/wish-lists/:id/items/:itemId", h.DeleteItem)

	cases := []struct{ method, path string }{
		{"GET", "/wish-lists/abc"},
		{"GET", "/wish-lists/0"},
		{"PUT", "/wish-lists/abc"},
		{"POST", "/wish-lists/abc/members"},
		{"DELETE", "/wish-lists/1/members/abc"},
		{"DELETE", "/wish-lists/1/members/-1"},
		{"GET", "/wish-lists/abc/items"},
		{"POST", "/wish-lists/abc/items"},
		{"PUT", "/wish-lists/1/items/abc"},
		{"DELETE", "/wish-lists/1/items/abc"},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(tc.method, tc.path, bytes.NewBufferString(`{}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s %s: expected 400, got %d", tc.method, tc.path, w.Code)
		}
	}
}

func TestBindWishList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/wish-lists", func(c *gin.Context) {
		name, ok := bindWishList(c)
		if !ok {
			return
		}
		c.JSON(http.StatusCreated, gin.H{"name": name})
	})

	cases := []struct {
		name string
		body string
		want int
	}{
		{"valid", `{"name":"Flat stuff"}`, http.StatusCreated},
		{"at limit", `{"name":"` + strings.Repeat("a", 64) + `"}`, http.StatusCreated},
		{"too long", `{"name":"` + strings.Repeat("a", 65) + `"}`, http.StatusBadRequest},
		{"empty body", `{}`, http.StatusBadRequest},
		{"whitespace", `{"name":"   "}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/wish-lists", bytes.NewBufferString(tc.body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Errorf("%s: expected %d, got %d (%s)", tc.name, tc.want, w.Code, w.Body.String())
		}
	}
}

func TestWishListMemberRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/members", func(c *gin.Context) {
		var req models.WishListMemberRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		c.Status(http.StatusOK)
	})

	cases := map[string]int{
		`{"user_id":2}`:  http.StatusOK,
		`{}`:             http.StatusBadRequest,
		`{"user_id":0}`:  http.StatusBadRequest,
		`{"user_id":-3}`: http.StatusBadRequest,
	}
	for body, want := range cases {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/members", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != want {
			t.Errorf("%s: expected %d, got %d", body, want, w.Code)
		}
	}
}
