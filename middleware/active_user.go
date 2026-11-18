package middleware

import (
	"database/sql"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"gopherdebt/db"
	"gopherdebt/models"
)

// ActiveUser runs after AuthMiddleware. A valid signature isn't enough: the account must still exist,
// not be deleted or blacklisted, and the token must be from its current token_version (a password
// change or deletion bumps it). Otherwise 401, which the frontend treats as "session expired".
func ActiveUser(database *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user_id := c.GetInt("userID")
		version, active, err := db.GetSessionState(database, user_id)
		if err != nil && err != db.ErrNotFound {
			log.Printf("ERROR ActiveUser: user %d: %v", user_id, err)
			c.JSON(http.StatusServiceUnavailable, models.APIResponse{Success: false, Error: "Service temporarily unavailable"})
			c.Abort()
			return
		}
		if err == db.ErrNotFound || !active || version != c.GetInt("tokenVersion") {
			c.JSON(http.StatusUnauthorized, models.APIResponse{Success: false, Error: "Session is no longer valid. Please log in again."})
			c.Abort()
			return
		}
		c.Next()
	}
}
