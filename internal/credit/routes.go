package credit

import (
	"github.com/cristianrisueo/gopay/internal/auth"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers all credit-related routes
func RegisterRoutes(router *gin.RouterGroup, creditService Service, authService *auth.AuthService) {
	handler := NewCreditHandler(creditService)

	// Protected routes (requires JWT) - Credit operations
	creditGroup := router.Group("/credit")
	creditGroup.Use(auth.AuthMiddleware(authService))
	{
		creditGroup.POST("/request", handler.RequestCredit)
		creditGroup.GET("/:id", handler.GetCredit)
		creditGroup.GET("/my-credits", handler.ListMyCredits)
		creditGroup.POST("/:id/accept", handler.AcceptCredit)
		creditGroup.POST("/:id/payment", handler.MakePayment)
		creditGroup.GET("/history", handler.GetCreditHistory)
	}
}
