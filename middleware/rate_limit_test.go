package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func newLimitedRouter() *gin.Engine {
	r := gin.New()
	r.SetTrustedProxies(nil)
	r.TrustedPlatform = "Fly-Client-IP"
	r.GET("/t", NewRateLimiter(2, time.Minute).Middleware(), func(c *gin.Context) { c.Status(200) })
	return r
}

func TestRateLimitIgnoresSpoofedForwardedFor(t *testing.T) {
	r := newLimitedRouter()
	codes := []int{}
	for _, ip := range []string{"1.1.1.1", "1.1.1.2", "1.1.1.3"} {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/t", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		req.Header.Set("X-Forwarded-For", ip)
		r.ServeHTTP(w, req)
		codes = append(codes, w.Code)
	}
	if codes[2] != 429 {
		t.Fatalf("want 3rd request rate limited despite new X-Forwarded-For, got %v", codes)
	}
}

func TestRateLimitUsesFlyClientIP(t *testing.T) {
	r := newLimitedRouter()
	for i, ip := range []string{"2.2.2.2", "2.2.2.2", "3.3.3.3"} {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/t", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		req.Header.Set("Fly-Client-IP", ip)
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("request %d from %s: want 200 (distinct Fly clients), got %d", i, ip, w.Code)
		}
	}
}
