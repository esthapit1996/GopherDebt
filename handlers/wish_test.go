package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestValidWishLink(t *testing.T) {
	cases := map[string]bool{
		"":                              true,
		"https://example.com/item?id=1": true,
		"http://shop.example.com":       true,
		"javascript:alert(1)":           false,
		"data:text/html,hi":             false,
		"ftp://example.com/file":        false,
		"example.com/no-scheme":         false,
		"https://":                      false,
	}
	for link, want := range cases {
		if got := validWishLink(link); got != want {
			t.Errorf("validWishLink(%q) = %v, want %v", link, got, want)
		}
	}
}

func wishBindRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/wish", func(c *gin.Context) {
		req, ok := bindWishItem(c)
		if !ok {
			return
		}
		c.JSON(http.StatusCreated, req)
	})
	return r
}

func TestBindWishItem(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"valid all fields", `{"name":"Switch 2","why":"Mario Kart","link":"https://example.com/switch","cost":449.99}`, http.StatusCreated},
		{"name only", `{"name":"Switch 2"}`, http.StatusCreated},
		{"null cost", `{"name":"Switch 2","cost":null}`, http.StatusCreated},
		{"empty body", `{}`, http.StatusBadRequest},
		{"missing name", `{"cost":10}`, http.StatusBadRequest},
		{"whitespace name", `{"name":"   ","cost":10}`, http.StatusBadRequest},
		{"zero cost", `{"name":"Switch 2","cost":0}`, http.StatusBadRequest},
		{"negative cost", `{"name":"Switch 2","cost":-5}`, http.StatusBadRequest},
		{"javascript link", `{"name":"x","link":"javascript:alert(1)"}`, http.StatusBadRequest},
		{"name at limit", `{"name":"` + strings.Repeat("a", 64) + `"}`, http.StatusCreated},
		{"name at limit multibyte", `{"name":"` + strings.Repeat("è", 64) + `"}`, http.StatusCreated},
		{"name too long", `{"name":"` + strings.Repeat("a", 65) + `"}`, http.StatusBadRequest},
		{"why at limit", `{"name":"x","why":"` + strings.Repeat("a", 128) + `"}`, http.StatusCreated},
		{"why too long", `{"name":"x","why":"` + strings.Repeat("a", 129) + `"}`, http.StatusBadRequest},
		{"link too long", `{"name":"x","link":"https://e.com/` + strings.Repeat("a", 2048) + `"}`, http.StatusBadRequest},
	}
	r := wishBindRouter()
	for _, tc := range cases {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/wish", bytes.NewBufferString(tc.body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Errorf("%s: expected %d, got %d (%s)", tc.name, tc.want, w.Code, w.Body.String())
		}
	}
}

func TestWishHandler_InvalidID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewWishHandler(nil)
	r := gin.New()
	r.PUT("/wish/:id", h.UpdateWishItem)
	r.DELETE("/wish/:id", h.DeleteWishItem)

	for _, method := range []string{"PUT", "DELETE"} {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(method, "/wish/abc", bytes.NewBufferString(`{"name":"x","cost":1}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400 for non-numeric wish id, got %d", method, w.Code)
		}
	}
}
