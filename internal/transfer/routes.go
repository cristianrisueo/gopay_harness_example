package transfer

import (
	"github.com/cristianrisueo/gopay/internal/auth"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers all transfer-related routes
func RegisterRoutes(router *gin.RouterGroup, transferService *TransferService, authService *auth.AuthService) {
	handler := NewTransferHandler(transferService)

	// Protected routes (requires JWT) - Transfer operations
	transferGroup := router.Group("/transfer")
	transferGroup.Use(auth.AuthMiddleware(authService))
	{
		transferGroup.POST("", handler.CreateTransfer)
		transferGroup.GET("/:id", handler.GetTransferByID)
		transferGroup.GET("/history", handler.GetTransferHistory)
	}
}
