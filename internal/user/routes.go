package user

import (
	"github.com/cristianrisueo/gopay/internal/auth"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers all user-related routes
func RegisterRoutes(router *gin.RouterGroup, userService *UserService, authService *auth.AuthService) {
	handler := NewUserHandler(userService, authService)

	// Public routes - Authentication
	authGroup := router.Group("/auth")
	{
		authGroup.POST("/register", handler.Register)
		authGroup.POST("/login", handler.Login)
	}

	// Protected routes (requires JWT) - User management
	usersGroup := router.Group("/users")
	usersGroup.Use(auth.AuthMiddleware(authService))
	{
		usersGroup.GET("/me", handler.GetProfile)
		usersGroup.PUT("/me", handler.UpdateProfile)
		usersGroup.GET("/search", handler.SearchUsers)
		usersGroup.GET("/:id", handler.GetUserByID)
		usersGroup.DELETE("/me", handler.DeleteAccount)
	}
}
