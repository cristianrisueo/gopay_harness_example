package user

import (
	"context"
	"errors"
	"fmt"

	"github.com/cristianrisueo/gopay/internal/wallet"
	"golang.org/x/crypto/bcrypt"
)

// Service defines the interface for user operations
type Service interface {
	Register(ctx context.Context, username, email, password string) (*User, error)
	Login(ctx context.Context, email, password string) (*User, error)
	GetByID(ctx context.Context, userID string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	Search(ctx context.Context, query string) ([]User, error)
	Update(ctx context.Context, userID, username, email string) (*User, error)
	Delete(ctx context.Context, userID string) error
}

// Type assertion to ensure UserService implements Service
var _ Service = (*UserService)(nil)

// UserService handles user business logic
type UserService struct {
	userRepo   Repository
	walletRepo wallet.Repository
}

// NewService creates a new user UserService instance
func NewService(userRepo Repository, walletRepo wallet.Repository) *UserService {
	return &UserService{
		userRepo:   userRepo,
		walletRepo: walletRepo,
	}
}

// Register creates a new user account
func (us *UserService) Register(ctx context.Context, username, email, password string) (*User, error) {
	// Checks if email already exists
	emailExists, err := us.userRepo.ExistsByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("failed to check email existence: %w", err)
	}

	if emailExists {
		return nil, ErrEmailExists
	}

	// Checks if username already exists
	usernameExists, err := us.userRepo.ExistsByUsername(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("failed to check username existence: %w", err)
	}

	if usernameExists {
		return nil, ErrUsernameExists
	}

	// Hashes the password
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Creates the user in the database
	user, err := us.userRepo.Create(ctx, username, email, string(passwordHash))
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Creates the wallet for the new user. On error deletes the created user
	_, err = us.walletRepo.Create(ctx, user.ID)
	if err != nil {
		// Rollback: delete the user if wallet creation fails
		deleteErr := us.userRepo.Delete(ctx, user.ID)
		if deleteErr != nil {
			return nil, fmt.Errorf(
				"failed to create wallet for user (rollback also failed: %v): %w",
				deleteErr, err,
			)
		}

		return nil, fmt.Errorf("failed to create wallet for user: %w", err)
	}

	return user, nil
}

// Login authenticates a user with email and password
func (us *UserService) Login(ctx context.Context, email, password string) (*User, error) {
	// Gets the user by email
	user, err := us.userRepo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, errors.New("invalid email or password")
		}

		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Verifies the password. If ok returns the user retrieved
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return nil, errors.New("invalid email or password")
	}

	return user, nil
}

// GetByID retrieves a user by ID
func (us *UserService) GetByID(ctx context.Context, userID string) (*User, error) {
	user, err := us.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return user, nil
}

// GetByEmail retrieves a user by email
func (us *UserService) GetByEmail(ctx context.Context, email string) (*User, error) {
	user, err := us.userRepo.GetByEmail(ctx, email)
	if err != nil {
		return nil, err
	}

	return user, nil
}

// GetByUsername retrieves a user by username
func (us *UserService) GetByUsername(ctx context.Context, username string) (*User, error) {
	user, err := us.userRepo.GetByUsername(ctx, username)
	if err != nil {
		return nil, err
	}

	return user, nil
}

// Search finds users by username or email
func (us *UserService) Search(ctx context.Context, query string) ([]User, error) {
	users, err := us.userRepo.Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to search users: %w", err)
	}

	return users, nil
}

// Update updates user information (username and email, assumes input is already validated by handler)
func (us *UserService) Update(ctx context.Context, userID, username, email string) (*User, error) {
	// Gets the user by ID
	existingUser, err := us.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Checks if new email is different and is not already taken by another user
	if email != existingUser.Email {
		emailExists, err := us.userRepo.ExistsByEmail(ctx, email)
		if err != nil {
			return nil, fmt.Errorf("failed to check email existence: %w", err)
		}

		if emailExists {
			return nil, ErrEmailExists
		}
	}

	// Check if new username  is different and is not already taken by another user
	if username != existingUser.Username {
		usernameExists, err := us.userRepo.ExistsByUsername(ctx, username)
		if err != nil {
			return nil, fmt.Errorf("failed to check username existence: %w", err)
		}

		if usernameExists {
			return nil, ErrUsernameExists
		}
	}

	// Updates the user
	user, err := us.userRepo.Update(ctx, userID, username, email)
	if err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	return user, nil
}

// Delete performs a soft delete of a user account
func (us *UserService) Delete(ctx context.Context, userID string) error {
	// Gets the user by ID
	_, err := us.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	// Check if all wallet balances are zero before deletion
	userWallet, err := us.walletRepo.GetByUserID(ctx, userID)

	if err != nil {
		// If the wallet is not found something strange happens, better not delete
		if !errors.Is(err, wallet.ErrWalletNotFound) {
			return fmt.Errorf("failed to get wallet: %w", err)
		}
	} else {
		// If wallet exists, verifies that all balances are zero
		if !userWallet.BalanceEUR.IsZero() ||
			!userWallet.BalanceUSD.IsZero() ||
			!userWallet.BalanceJPY.IsZero() {
			return errors.New("cannot delete user with non-zero balances")
		}
	}

	// If the wallets exists and its balances are zero perform soft delete
	err = us.userRepo.Delete(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	return nil
}
