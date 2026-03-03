package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type visitor struct {
	count       int
	windowStart time.Time
}

// RateLimiter enforces a per-IP request limit within a sliding time window.
type RateLimiter struct {
	visitors map[string]*visitor
	mu       sync.Mutex
	limit    int
	window   time.Duration
}

// NewRateLimiter creates a rate limiter that allows `limit` requests per `window` per IP.
// A background goroutine cleans up stale entries every minute.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		limit:    limit,
		window:   window,
	}
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) cleanup() {
	for {
		time.Sleep(time.Minute)
		rl.mu.Lock()
		for ip, v := range rl.visitors {
			if time.Since(v.windowStart) > rl.window {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// Middleware returns a Gin middleware that enforces the rate limit.
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()

		rl.mu.Lock()
		v, exists := rl.visitors[ip]
		if !exists || time.Since(v.windowStart) > rl.window {
			// New window
			rl.visitors[ip] = &visitor{count: 1, windowStart: time.Now()}
			rl.mu.Unlock()
			c.Next()
			return
		}

		if v.count >= rl.limit {
			rl.mu.Unlock()
			c.JSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"error":   "Too many requests. Please try again later.",
			})
			c.Abort()
			return
		}

		v.count++
		rl.mu.Unlock()
		c.Next()
	}
}
