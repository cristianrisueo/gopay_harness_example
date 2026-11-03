package staking

import (
	"github.com/cristianrisueo/gopay/internal/auth"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers all staking-related routes
func RegisterRoutes(router *gin.RouterGroup, stakingService *StakingService, authService *auth.AuthService) {
	handler := NewStakingHandler(stakingService)

	// Protected routes (requires JWT) - Staking operations
	stakingGroup := router.Group("/staking")
	stakingGroup.Use(auth.AuthMiddleware(authService))
	{
		stakingGroup.POST("/stake", handler.CreateStake)
		stakingGroup.POST("/:id/increase", handler.IncreaseStake)
		stakingGroup.GET("/positions", handler.GetPositions)
		stakingGroup.POST("/:id/withdraw", handler.WithdrawStake)
	}
}
