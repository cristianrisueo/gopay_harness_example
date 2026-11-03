package wallet

import (
	"github.com/cristianrisueo/gopay/internal/auth"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers all wallet-related routes
func RegisterRoutes(router *gin.RouterGroup, walletService *WalletService, authService *auth.AuthService) {
	handler := NewWalletHandler(walletService)

	// Protected routes (requires JWT) - Wallet operations
	walletGroup := router.Group("/wallet")
	walletGroup.Use(auth.AuthMiddleware(authService))
	{
		walletGroup.GET("/balance", handler.GetBalance)
		walletGroup.POST("/deposit", handler.ProcessTransaction(walletService.DepositWithKey))
		walletGroup.POST("/withdraw", handler.ProcessTransaction(walletService.WithdrawWithKey))
	}
}
