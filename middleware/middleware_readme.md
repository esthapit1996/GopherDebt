# 🔐 Middleware (`middleware/`)

Middleware functions run **before** your handlers. They can validate requests, add data to the context, or reject invalid requests.

---

## 📚 Libraries Used

| Library | Import Path | Purpose |
|---------|-------------|---------|
| `gin-gonic/gin` | `github.com/gin-gonic/gin` | Web framework - provides middleware interface |
| `golang-jwt/jwt` | `github.com/golang-jwt/jwt/v5` | Parse and validate JWT tokens |

---

## 📁 Files

| File | Purpose |
|------|---------|
| `auth.go` | JWT authentication middleware |
| `rate_limit.go` | Per-IP rate limiting middleware |
| `auth_test.go` | Authentication middleware tests |

---

## 🔧 auth.go

### Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `AuthMiddleware()` | `gin.HandlerFunc` | Creates middleware that validates JWT tokens |

### How It Works

```
1. Request comes in with header: "Authorization: Bearer <token>"
2. Middleware extracts the token string
3. Parses and validates the JWT signature using JWT_SECRET
4. Extracts user_id from token claims
5. Sets userID in Gin context: c.Set("userID", userID)
6. Calls c.Next() to proceed to handler
```

### Flow Diagram

```
Request → [AuthMiddleware] → Handler
              ↓
         Check header
              ↓
         Parse "Bearer <token>"
              ↓
         Validate JWT signature
              ↓
         Extract user_id claim
              ↓
         c.Set("userID", id)
              ↓
         c.Next() → Handler
```

### Error Responses

| Condition | HTTP Status | Error Message |
|-----------|-------------|---------------|
| No Authorization header | 401 | "Authorization header required" |
| Not "Bearer <token>" format | 401 | "Invalid authorization header format" |
| Invalid/expired JWT | 401 | "Invalid or expired token" |
| Missing user_id in claims | 401 | "Invalid user ID in token" |

### Environment Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `JWT_SECRET` | `"your-secret-key-change-in-production"` | Secret key for signing JWTs |

### Usage in Handlers

```go
// In any protected handler:
userID := c.GetInt("userID")  // Get the authenticated user's ID
```

---

## 🔄 How Middleware is Applied

In `main.go`:

```go
// Protected routes - require valid JWT
protected := r.Group("/api")
protected.Use(middleware.AuthMiddleware())
{
    protected.GET("/me", userHandler.GetProfile)
    protected.GET("/groups", groupHandler.GetGroups)
    // ... all protected routes
}
```

Routes inside the `protected` group will **automatically** run through `AuthMiddleware()` before reaching their handlers.

---

## 🔒 JWT Token Structure

```json
{
  "user_id": 123,
  "exp": 1709337600,  // Expiration timestamp (24h from login)
  "iat": 1709251200   // Issued at timestamp
}
```

The token is signed with HMAC-SHA256 using `JWT_SECRET`.

---

## 🔧 rate_limit.go

### Struct

```go
type RateLimiter struct {
    visitors map[string]*visitor  // Per-IP request tracking
    mu       sync.Mutex           // Thread-safe access
    limit    int                  // Max requests per window
    window   time.Duration        // Time window
}
```

### Functions

| Function | Parameters | Returns | Description |
|----------|------------|---------|-------------|
| `NewRateLimiter` | `limit int, window time.Duration` | `*RateLimiter` | Creates rate limiter with background cleanup goroutine |
| `Middleware()` | — | `gin.HandlerFunc` | Returns Gin middleware that enforces per-IP rate limit |

### How It Works

```
1. Request comes in from IP address
2. Check if IP has an active window
3. If new IP or window expired → start fresh window (count = 1)
4. If count < limit → increment count, allow request
5. If count >= limit → return 429 Too Many Requests
6. Background goroutine cleans stale entries every minute
```

### Usage in main.go

```go
authLimiter := middleware.NewRateLimiter(10, time.Minute)   // Strict: 10/min for login/register
apiLimiter := middleware.NewRateLimiter(100, time.Minute)    // Normal: 100/min for API routes

r.POST("/api/register", authLimiter.Middleware(), userHandler.Register)
r.POST("/api/login", authLimiter.Middleware(), userHandler.Login)

api := r.Group("/api")
api.Use(middleware.AuthMiddleware())
api.Use(apiLimiter.Middleware())
```
