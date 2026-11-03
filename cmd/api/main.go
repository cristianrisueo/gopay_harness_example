package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cristianrisueo/gopay/pkg/app"
	"github.com/cristianrisueo/gopay/pkg/database"
	"github.com/cristianrisueo/gopay/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"go.uber.org/zap"
)

func main() {
	// Loads .env file or use system environment variables
	if err := godotenv.Load(); err != nil {
		fmt.Println("Warning: .env file not found, using system environment variables")
	}

	// Load environment configuration
	env := app.GetEnv("ENV", "development")
	port := app.GetEnv("PORT", "8080")
	dsn := app.GetEnv("DB_DSN", "postgres://admin:password@localhost:5432/gopay?sslmode=disable")

	// Initializes the logger
	log := logger.New(env)
	defer log.Sync()

	// Connects to the database
	db, err := database.New(dsn)
	if err != nil {
		log.Fatal("failed to connect to database", zap.Error(err))
	}

	defer db.Close()
	log.Info("database connection established")

	// Sets Gin mode based on environment
	if env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Sets up HTTP routes and handlers
	router := setupRouter(db, log)

	// Configures HTTP server
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	// Starts the server and waits for shutdown signal
	app.StartServer(srv, log, port)
	app.WaitForShutdown(srv, log, 5*time.Second)
}
