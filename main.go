package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

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
		connStr = "postgresql://postgres.rbcewoduprlgwydffiyz:Ganzgenau12345*@aws-1-eu-west-1.pooler.supabase.com:5432/postgres"
	}

	database, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Error opening database:", err)
	}
	defer database.Close()

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

	// Setup router
	r := gin.Default()

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "healthy"})
	})

	// Public routes
	r.POST("/api/register", userHandler.Register)
	r.POST("/api/login", userHandler.Login)

	// Protected routes
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	{
		// User routes
		api.GET("/profile", userHandler.GetProfile)
		api.GET("/users", userHandler.GetAllUsers)

		// Group routes
		api.POST("/groups", groupHandler.CreateGroup)
		api.GET("/groups", groupHandler.GetMyGroups)
		api.GET("/groups/:id", groupHandler.GetGroup)
		api.DELETE("/groups/:id", groupHandler.DeleteGroup)
		api.POST("/groups/:id/members", groupHandler.AddMember)
		api.DELETE("/groups/:id/members/:memberID", groupHandler.RemoveMember)

		// Expense routes
		api.POST("/groups/:id/expenses", expenseHandler.CreateExpense)
		api.GET("/groups/:id/expenses", expenseHandler.GetGroupExpenses)
		api.GET("/groups/:id/expenses/:expenseID", expenseHandler.GetExpense)
		api.DELETE("/groups/:id/expenses/:expenseID", expenseHandler.DeleteExpense)

		// Settlement routes
		api.POST("/groups/:id/settlements", settlementHandler.CreateSettlement)
		api.GET("/groups/:id/settlements", settlementHandler.GetGroupSettlements)

		// Balance routes
		api.GET("/groups/:id/balances", settlementHandler.GetGroupBalances)
		api.GET("/groups/:id/my-balance", settlementHandler.GetMyBalance)
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
