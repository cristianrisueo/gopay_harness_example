package main

import (
	"database/sql"
	"net/http"

	"github.com/cristianrisueo/gopay/internal/auth"
	"github.com/cristianrisueo/gopay/internal/credit"
	"github.com/cristianrisueo/gopay/internal/pool"
	"github.com/cristianrisueo/gopay/internal/staking"
	"github.com/cristianrisueo/gopay/internal/transfer"
	"github.com/cristianrisueo/gopay/internal/user"
	"github.com/cristianrisueo/gopay/internal/wallet"
	"github.com/cristianrisueo/gopay/pkg/app"
	"github.com/cristianrisueo/gopay/pkg/llm"
	"github.com/cristianrisueo/gopay/pkg/response"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// setupRouter configures all HTTP dependencies and returns the router
func setupRouter(db *sql.DB, log *zap.Logger) *gin.Engine {
	router := gin.Default()

	// Loads configuration
	jwtConfig := app.LoadJWTConfig()
	llmConfig := app.LoadLLMConfig()

	// Initializes auth service
	authService := auth.NewAuthService(jwtConfig.Secret, jwtConfig.ExpirationHours)

	// Initializes wallet repository and service
	walletRepo := wallet.NewPostgresRepository(db)
	walletService := wallet.NewService(walletRepo)

	// Initializes user repository and service
	userRepo := user.NewPostgresRepository(db)
	userService := user.NewService(userRepo, walletRepo)

	// Initializes transfer repository and service
	transferRepo := transfer.NewPostgresRepository(db)
	transferService := transfer.NewService(transferRepo, walletRepo, log)

	// Initializes liquidity pool repository and service
	poolRepo := pool.NewPostgresRepository(db)
	poolService := pool.NewService(poolRepo, log)

	// Initializes staking repository and service
	stakingRepo := staking.NewPostgresRepository(db)
	stakingService := staking.NewService(stakingRepo, walletService, poolService)

	// Initializes LLM client for credit evaluation
	llmClient := llm.New(llmConfig.APIKey, llmConfig.Model, log)

	// Initializes credit repository and service
	creditRepo := credit.NewPostgresRepository(db)
	creditService := credit.NewService(creditRepo, userRepo, walletService, poolService, llmClient, log)

	// Registers the routes routes
	router.GET("/health", healthCheckHandler(db))                           // Health route
	user.RegisterRoutes(router.Group(""), userService, authService)         // User routes
	wallet.RegisterRoutes(router.Group(""), walletService, authService)     // Wallet routes
	transfer.RegisterRoutes(router.Group(""), transferService, authService) // Transfer routes
	pool.RegisterRoutes(router.Group(""), poolService, authService)         // Pool routes
	staking.RegisterRoutes(router.Group(""), stakingService, authService)   // Staking routes
	credit.RegisterRoutes(router.Group(""), creditService, authService)     // Credit routes

	return router
}

// healthCheckHandler returns a handler that verifies database connectivity
func healthCheckHandler(db *sql.DB) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if err := db.Ping(); err != nil {
			response.InternalServerError(ctx, "database health check failed")
			return
		}

		response.Success(ctx, http.StatusOK, gin.H{
			"status":  "healthy",
			"service": "gopay-api",
			"version": "3.0.0",
		})
	}
}
