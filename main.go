package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"

	"gopherdebt/db"
	"gopherdebt/handlers"
	"gopherdebt/middleware"
)

func main() {
	// Database connection
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		dbPassword := os.Getenv("DB_PASSWORD")
		if dbPassword == "" {
			log.Fatal("DB_PASSWORD environment variable is required")
		}
		connStr = fmt.Sprintf("host=aws-1-eu-west-1.pooler.supabase.com port=5432 user=postgres.rbcewoduprlgwydffiyz password=%s dbname=postgres sslmode=require connect_timeout=5 keepalives=1 keepalives_idle=30 keepalives_interval=10 keepalives_count=3", dbPassword)
	} else {
		// Append TCP keepalives to DATABASE_URL if not already present
		if !strings.Contains(connStr, "keepalives") {
			connStr += "?keepalives=1&keepalives_idle=30&keepalives_interval=10&keepalives_count=3&connect_timeout=5"
		}
	}

	database, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Error opening database:", err)
	}
	defer database.Close()

	// Connection pool settings — aggressive recycling to avoid stale PgBouncer connections
	database.SetMaxOpenConns(10)
	database.SetMaxIdleConns(3)
	database.SetConnMaxLifetime(3 * time.Minute)
	database.SetConnMaxIdleTime(30 * time.Second)

	// Test connection
	if err := database.Ping(); err != nil {
		log.Fatal("Could not reach database:", err)
	}
	fmt.Println("GopherDebt is ONLINE!")

	// Run migrations
	if err := db.RunMigrations(database); err != nil {
		log.Fatal("Migration failed:", err)
	}

	// Initialize handlers
	userHandler := handlers.NewUserHandler(database)
	groupHandler := handlers.NewGroupHandler(database)
	expenseHandler := handlers.NewExpenseHandler(database)
	settlementHandler := handlers.NewSettlementHandler(database)
	expensePaymentHandler := handlers.NewExpensePaymentHandler(database)
	activityHandler := handlers.NewActivityHandler(database)
	suggestionHandler := handlers.NewSuggestionHandler(database)
	currencyHandler := handlers.NewCurrencyHandler()
	accessControlHandler := handlers.NewAccessControlHandler(database)
	receiptHandler := handlers.NewReceiptHandler()
	stashHandler := handlers.NewStashHandler(database)

	// Setup router
	r := gin.Default()

	// CORS middleware and cache prevention
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, Cache-Control")
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// Rate limiters: strict for auth, normal for protected endpoints
	authLimiter := middleware.NewRateLimiter(10, time.Minute) // 10 req/min for login/register
	apiLimiter := middleware.NewRateLimiter(100, time.Minute) // 100 req/min for API routes

	// Root endpoint
	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"name":    "GopherDebt API",
			"version": "1.0.0",
			"status":  "running",
			"endpoints": gin.H{
				"health":   "GET /health",
				"register": "POST /api/register",
				"login":    "POST /api/login",
				"docs":     "See /api/* for protected endpoints",
			},
		})
	})

	// Admin: Reset all financial data (keep users and groups)
	r.POST("/admin/reset-finances", func(c *gin.Context) {
		database.Exec("TRUNCATE settlements, expense_payments, expense_splits, expenses RESTART IDENTITY CASCADE")
		c.JSON(200, gin.H{"message": "All expenses, payments, and settlements cleared"})
	})

	// Health check — actually ping DB so we detect stale connections
	r.GET("/health", func(c *gin.Context) {
		if err := database.Ping(); err != nil {
			c.JSON(503, gin.H{"status": "unhealthy", "error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"status": "healthy"})
	})

	// Public routes (strict rate limit to prevent brute force)
	r.POST("/api/register", authLimiter.Middleware(), userHandler.Register)
	r.POST("/api/login", authLimiter.Middleware(), userHandler.Login)

	// Protected routes
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	api.Use(apiLimiter.Middleware())
	{
		// User routes
		api.GET("/profile", userHandler.GetProfile)
		api.GET("/users", userHandler.GetAllUsers)
		api.DELETE("/users/:id", userHandler.DeleteUser)
		api.GET("/debt-overview", userHandler.GetDebtOverview)
		api.GET("/debt-overview/:userId", userHandler.GetDebtDetails)
		api.GET("/payment-history", userHandler.GetPaymentHistory)
		api.DELETE("/payment-history", userHandler.ClearPaymentHistory)
		api.PUT("/profile/theme", userHandler.UpdateTheme)
		api.PUT("/profile/avatar", userHandler.UpdateAvatar)
		api.PUT("/profile/language", userHandler.UpdateLanguage)
		api.PUT("/profile/password", userHandler.ChangePassword)

		// Group routes
		api.POST("/groups", groupHandler.CreateGroup)
		api.GET("/groups", groupHandler.GetMyGroups)
		api.GET("/groups/:id", groupHandler.GetGroup)
		api.PUT("/groups/:id", groupHandler.UpdateGroup)
		api.DELETE("/groups/:id", groupHandler.DeleteGroup)
		api.POST("/groups/:id/members", groupHandler.AddMember)
		api.DELETE("/groups/:id/members/:memberID", groupHandler.RemoveMember)

		// Expense routes
		api.POST("/groups/:id/expenses", expenseHandler.CreateExpense)
		api.PUT("/groups/:id/expenses/:expenseID", expenseHandler.UpdateExpense)
		api.GET("/groups/:id/expenses", expenseHandler.GetGroupExpenses)
		api.GET("/groups/:id/expenses/unpaid", expenseHandler.GetUnpaidExpenses)
		api.GET("/groups/:id/expenses/:expenseID", expenseHandler.GetExpense)
		api.DELETE("/groups/:id/expenses/:expenseID", expenseHandler.DeleteExpense)
		api.DELETE("/groups/:id/expenses", expenseHandler.ClearAllExpenses)

		// Expense payment routes (partial repayments)
		api.POST("/expenses/:expenseId/payments", expensePaymentHandler.CreateExpensePayment)
		api.GET("/expenses/:expenseId/payments", expensePaymentHandler.GetExpensePayments)
		api.DELETE("/payments/:paymentId", expensePaymentHandler.DeleteExpensePayment)
		api.GET("/groups/:id/expense-payment-statuses", expensePaymentHandler.GetGroupExpensePaymentStatuses)

		// Settlement routes
		api.POST("/groups/:id/settlements", settlementHandler.CreateSettlement)
		api.GET("/groups/:id/settlements", settlementHandler.GetGroupSettlements)
		api.DELETE("/groups/:id/settlements/:settlementID", settlementHandler.DeleteSettlement)

		// Balance routes
		api.GET("/groups/:id/balances", settlementHandler.GetGroupBalances)
		api.GET("/groups/:id/my-balance", settlementHandler.GetMyBalance)

		// Activity routes
		api.GET("/groups/:id/activities", activityHandler.GetGroupActivities)

		// Suggestion routes
		api.GET("/suggestions", suggestionHandler.GetSuggestions)
		api.POST("/suggestions", suggestionHandler.CreateSuggestion)
		api.PUT("/suggestions/:id", suggestionHandler.EditSuggestion)
		api.DELETE("/suggestions/:id", suggestionHandler.DeleteSuggestion)
		api.POST("/suggestions/:id/vote", suggestionHandler.VoteSuggestion)
		api.DELETE("/suggestions/:id/vote", suggestionHandler.RemoveVote)
		api.GET("/suggestions/:id/voters", suggestionHandler.GetVoters)
		api.PUT("/suggestions/:id/status", suggestionHandler.UpdateSuggestionStatus)
		api.GET("/suggestions/:id/comments", suggestionHandler.GetComments)
		api.POST("/suggestions/:id/comments", suggestionHandler.CreateComment)
		api.PUT("/suggestions/:id/comments/:commentId", suggestionHandler.EditComment)
		api.DELETE("/suggestions/:id/comments/:commentId", suggestionHandler.DeleteComment)

		// Currency routes
		api.GET("/currency/rates", currencyHandler.GetRates)
		api.GET("/currency/convert", currencyHandler.Convert)
		api.GET("/currency/history", currencyHandler.GetHistory)

		// Receipt scanning (Gemini proxy)
		api.POST("/receipt/scan", receiptHandler.ScanReceipt)

		// GopherStash routes (personal expense tracker)
		api.GET("/stash", stashHandler.GetStashExpenses)
		api.POST("/stash", stashHandler.CreateStashExpense)
		api.DELETE("/stash/:id", stashHandler.DeleteStashExpense)
		api.GET("/stash/summary", stashHandler.GetStashSummary)
		api.DELETE("/stash", stashHandler.ClearStashExpenses)

		// Access control routes (whitelist/blacklist)
		api.GET("/whitelist", accessControlHandler.GetWhitelist)
		api.POST("/whitelist", accessControlHandler.AddToWhitelist)
		api.DELETE("/whitelist/:id", accessControlHandler.RemoveFromWhitelist)
		api.GET("/blacklist", accessControlHandler.GetBlacklist)
		api.POST("/blacklist", accessControlHandler.AddToBlacklist)
		api.DELETE("/blacklist/:id", accessControlHandler.RemoveFromBlacklist)
	}

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Printf("Server starting on port %s...\n", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
