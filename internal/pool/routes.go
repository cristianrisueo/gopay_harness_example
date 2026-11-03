package pool

import (
	"github.com/cristianrisueo/gopay/internal/auth"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers pool routes to the router
func RegisterRoutes(router *gin.RouterGroup, poolService *PoolService, authService *auth.AuthService) {
	handler := NewPoolHandler(poolService)

	// Pool routes (authentication required)
	poolGroup := router.Group("/pool")
	poolGroup.Use(auth.AuthMiddleware(authService))
	{
		poolGroup.GET("/stats", handler.GetAllPoolStats)
		poolGroup.GET("/stats/:currency", handler.GetPoolStats)
		poolGroup.GET("/interest-history/:currency", handler.GetInterestHistory)
	}
}
