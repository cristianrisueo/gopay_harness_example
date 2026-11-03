package user

import (
	"errors"
	"time"
)

// Users domain custom errors
var (
	ErrUserNotFound   = errors.New("user not found")
	ErrEmailExists    = errors.New("email already exists")
	ErrUsernameExists = errors.New("username already exists")
)

// User represents the domain model for a user
type User struct {
	ID           string     `json:"id"`
	Username     string     `json:"username"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
}

// RegisterRequest represents the input for user registration
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

// LoginRequest represents the input for user login
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// UpdateRequest represents the input for updating user profile
type UpdateRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Email    string `json:"email" binding:"required,email"`
}

// SearchRequest represents the input for searching users
type SearchRequest struct {
	Query string `form:"q" binding:"required,min=2"`
}

// LoginResponse represents the output after successful login
type LoginResponse struct {
	Token string   `json:"token"`
	User  UserData `json:"user"`
}

// SearchResponse represents simplified user information for search results
type SearchResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

// UserData represents user information response
type UserData struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// ToUserData converts a User to ToUserData (removes sensitive data)
func (u *User) ToUserData() UserData {
	return UserData{
		ID:        u.ID,
		Username:  u.Username,
		Email:     u.Email,
		CreatedAt: u.CreatedAt,
	}
}

// ToSearchResult converts a User to a SearchResult
func (u *User) ToSearchResult() SearchResponse {
	return SearchResponse{
		ID:       u.ID,
		Username: u.Username,
		Email:    u.Email,
	}
}
