package user

import (
	"errors"
	"net/http"

	"github.com/cristianrisueo/gopay/internal/auth"
	"github.com/cristianrisueo/gopay/pkg/response"
	"github.com/gin-gonic/gin"
)

// UserHandler handles HTTP requests for user operations
type UserHandler struct {
	userService *UserService
	authService *auth.AuthService
}

// NewUserHandler creates a new user handler instance
func NewUserHandler(userService *UserService, authService *auth.AuthService) *UserHandler {
	return &UserHandler{
		userService: userService,
		authService: authService,
	}
}

// Register handles user registration
func (uh *UserHandler) Register(ctx *gin.Context) {
	// Validates the input data
	var req RegisterRequest

	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.BadRequest(ctx, "Invalid input: "+err.Error())
		return
	}

	// Creates the user
	user, err := uh.userService.Register(ctx.Request.Context(), req.Username, req.Email, req.Password)
	if err != nil {
		if errors.Is(err, ErrEmailExists) {
			response.ErrorResponse(ctx, http.StatusConflict, response.ErrCodeUserAlreadyExists, "Email already exists")
			return
		}

		if errors.Is(err, ErrUsernameExists) {
			response.ErrorResponse(ctx, http.StatusConflict, response.ErrCodeUserAlreadyExists, "Username already exists")
			return
		}

		response.InternalServerError(ctx, "Failed to register user")
		return
	}

	// Returns success response with token and user data
	response.Success(ctx, http.StatusCreated, user.ToUserData())
}

// Login handles user authentication
func (uh *UserHandler) Login(ctx *gin.Context) {
	// Validates the input data
	var req LoginRequest

	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.BadRequest(ctx, "Invalid input: "+err.Error())
		return
	}

	// Authenticates the user
	user, err := uh.userService.Login(ctx.Request.Context(), req.Email, req.Password)
	if err != nil {
		response.Unauthorized(ctx, "Invalid email or password")
		return
	}

	// Generates the JWT token from user's data
	token, err := uh.authService.GenerateToken(user.ID, user.Email, user.Username)
	if err != nil {
		response.InternalServerError(ctx, "Failed to generate token")
		return
	}

	// Returns success response with token and user data
	res := LoginResponse{
		Token: token,
		User:  user.ToUserData(),
	}

	response.Success(ctx, http.StatusOK, res)
}

// GetProfile retrieves the authenticated user's profile
func (uh *UserHandler) GetProfile(ctx *gin.Context) {
	// Gets user ID from context (set by AuthMiddleware)
	userID, exists := ctx.Get(auth.ContextKeyUserID)
	if !exists {
		response.Unauthorized(ctx, "User not authenticated")
		return
	}

	// Gets the user by ID
	user, err := uh.userService.GetByID(ctx.Request.Context(), userID.(string))
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			response.NotFound(ctx, "User not found")
			return
		}
		response.InternalServerError(ctx, "Failed to get user profile")
		return
	}

	// Returns success response with retrieved user data
	response.Success(ctx, http.StatusOK, user.ToUserData())
}

// GetUserByID retrieves another user's profile by ID
func (uh *UserHandler) GetUserByID(ctx *gin.Context) {
	userID := ctx.Param("id")

	// Gets the user by ID
	user, err := uh.userService.GetByID(ctx.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			response.NotFound(ctx, "User not found")
			return
		}

		response.InternalServerError(ctx, "Failed to get user")
		return
	}

	// Returns success response with retrieved user data
	response.Success(ctx, http.StatusOK, user.ToUserData())
}

// UpdateProfile updates the authenticated user's profile
func (uh *UserHandler) UpdateProfile(ctx *gin.Context) {
	// Validates the input data
	var req UpdateRequest

	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.BadRequest(ctx, "Invalid input: "+err.Error())
		return
	}

	// Gets user ID from context (set by AuthMiddleware)
	userID, exists := ctx.Get(auth.ContextKeyUserID)
	if !exists {
		response.Unauthorized(ctx, "User not authenticated")
		return
	}

	// Updates the user
	user, err := uh.userService.Update(ctx.Request.Context(), userID.(string), req.Username, req.Email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			response.NotFound(ctx, "User not found")
			return
		}

		if errors.Is(err, ErrEmailExists) {
			response.ErrorResponse(ctx, http.StatusConflict, response.ErrCodeUserAlreadyExists, "Email already exists")
			return
		}

		if errors.Is(err, ErrUsernameExists) {
			response.ErrorResponse(ctx, http.StatusConflict, response.ErrCodeUserAlreadyExists, "Username already exists")
			return
		}

		response.InternalServerError(ctx, "Failed to update user profile")
		return
	}

	// Returns success response with retrieved user data updated
	response.Success(ctx, http.StatusOK, user.ToUserData())
}

// DeleteAccount deletes the authenticated user's account
func (uh *UserHandler) DeleteAccount(ctx *gin.Context) {
	// Gets user ID from context (set by AuthMiddleware)
	userID, exists := ctx.Get(auth.ContextKeyUserID)
	if !exists {
		response.Unauthorized(ctx, "User not authenticated")
		return
	}

	// Deletes the user
	err := uh.userService.Delete(ctx.Request.Context(), userID.(string))
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			response.NotFound(ctx, "User not found")
			return
		}

		if err.Error() == "cannot delete user with non-zero balances" {
			response.ErrorResponse(ctx, http.StatusBadRequest, response.ErrCodeCannotDeleteUser, err.Error())
			return
		}

		response.InternalServerError(ctx, "Failed to delete account")
		return
	}

	// Returns success response with success message
	response.Success(ctx, http.StatusOK, gin.H{"message": "Account deleted successfully"})
}

// SearchUsers searches for users by username or email
func (uh *UserHandler) SearchUsers(ctx *gin.Context) {
	// Validates the input data
	var query SearchRequest

	if err := ctx.ShouldBindQuery(&query); err != nil {
		response.BadRequest(ctx, "Invalid query: "+err.Error())
		return
	}

	// Searches users
	users, err := uh.userService.Search(ctx.Request.Context(), query.Query)
	if err != nil {
		response.InternalServerError(ctx, "Failed to search users")
		return
	}

	// Converts users found to search results type
	results := make([]SearchResponse, len(users))
	for i, u := range users {
		results[i] = u.ToSearchResult()
	}

	// Returns success response with users found
	response.Success(ctx, http.StatusOK, results)
}
