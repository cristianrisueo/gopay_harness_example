package user

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Repository defines the interface for user data operations
type Repository interface {
	Create(ctx context.Context, username, email, passwordHash string) (*User, error)
	GetByID(ctx context.Context, userID string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	Search(ctx context.Context, query string) ([]User, error)
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	ExistsByUsername(ctx context.Context, username string) (bool, error)
	Update(ctx context.Context, userID, username, email string) (*User, error)
	Delete(ctx context.Context, userID string) error
}

// UsersRepository implements Repository using PostgreSQL
type UsersRepository struct {
	db *sql.DB
}

// Type assertion to ensure UsersRepository implements Repository
var _ Repository = (*UsersRepository)(nil)

// NewPostgresRepository creates a new PostgreSQL repository instance
func NewPostgresRepository(db *sql.DB) *UsersRepository {
	return &UsersRepository{db: db}
}

// Create inserts a new user into the database
func (ur *UsersRepository) Create(ctx context.Context, username, email, passwordHash string) (*User, error) {
	// Creates an empty user and the query to execute
	user := &User{}

	query := `
		INSERT INTO users (id, username, email, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		RETURNING id, username, email, password_hash, created_at, updated_at, deleted_at
	`

	// Executes the query and maps the result to the user. If everything's ok returns it
	err := ur.db.QueryRowContext(
		ctx,
		query,
		uuid.New().String(),
		username,
		email,
		passwordHash,
	).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.DeletedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

// GetByID retrieves a user by ID (excluding soft-deleted users)
func (ur *UsersRepository) GetByID(ctx context.Context, userID string) (*User, error) {
	// Creates an empty user and the query to execute
	user := &User{}

	query := `
		SELECT id, username, email, password_hash, created_at, updated_at, deleted_at
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
	`

	// Executes the query and maps the result to the user. If everything's ok returns it
	err := ur.db.QueryRowContext(ctx, query, userID).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.DeletedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}

		return nil, fmt.Errorf("failed to get user by id: %w", err)
	}

	return user, nil
}

// GetByEmail retrieves a user by email address (excluding soft-deleted users)
func (ur *UsersRepository) GetByEmail(ctx context.Context, email string) (*User, error) {
	// Creates an empty user and the query to execute
	user := &User{}

	query := `
		SELECT id, username, email, password_hash, created_at, updated_at, deleted_at
		FROM users
		WHERE email = $1 AND deleted_at IS NULL
	`

	// Executes the query and maps the result to the user. If everything's ok returns it
	err := ur.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.DeletedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}

		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	return user, nil
}

// GetByUsername retrieves a user by username (excluding soft-deleted users)
func (ur *UsersRepository) GetByUsername(ctx context.Context, username string) (*User, error) {
	// Creates an empty user and the query to execute
	user := &User{}

	query := `
		SELECT id, username, email, password_hash, created_at, updated_at, deleted_at
		FROM users
		WHERE username = $1 AND deleted_at IS NULL
	`

	// Executes the query and maps the result to the user. If everything's ok returns it
	err := ur.db.QueryRowContext(ctx, query, username).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.DeletedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}

		return nil, fmt.Errorf("failed to get user by username: %w", err)
	}

	return user, nil
}

// Search finds users by username or email with partial matching
func (ur *UsersRepository) Search(ctx context.Context, param string) ([]User, error) {
	// Creates the partial query and the query from it
	searchPattern := "%" + param + "%"

	query := `
		SELECT id, username, email, password_hash, created_at, updated_at, deleted_at
		FROM users
		WHERE (username ILIKE $1 OR email ILIKE $1) AND deleted_at IS NULL
		ORDER BY username
		LIMIT 50
	`

	// Executes the query
	rows, err := ur.db.QueryContext(ctx, query, searchPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to search users: %w", err)
	}

	defer rows.Close()

	// Scans each record found on the array of users. If everything's ok returns it
	var users []User
	for rows.Next() {
		var user User

		err := rows.Scan(
			&user.ID,
			&user.Username,
			&user.Email,
			&user.PasswordHash,
			&user.CreatedAt,
			&user.UpdatedAt,
			&user.DeletedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}

		users = append(users, user)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating users: %w", err)
	}

	return users, nil
}

// ExistsByEmail checks if a user exists by email (excluding soft-deleted)
func (ur *UsersRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	// Creates the bool and query
	var exists bool

	query := `
		SELECT EXISTS(
			SELECT 1 FROM users WHERE email = $1 AND deleted_at IS NULL
		)
	`

	// Executes the query and returns and scans the result in the bool. If ok returns it
	err := ur.db.QueryRowContext(ctx, query, email).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check email existence: %w", err)
	}

	return exists, nil
}

// ExistsByUsername checks if a a user exists by username (excluding soft-deleted)
func (ur *UsersRepository) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	// Creates the bool and query
	var exists bool

	query := `
		SELECT EXISTS(
			SELECT 1 FROM users WHERE username = $1 AND deleted_at IS NULL
		)
	`

	// Executes the query and returns and scans the result in the bool. If ok returns it
	err := ur.db.QueryRowContext(ctx, query, username).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check username existence: %w", err)
	}

	return exists, nil
}

// Update updates user information (username and email)
func (ur *UsersRepository) Update(ctx context.Context, userID, username, email string) (*User, error) {
	// Creates an empty user and the query to execute
	user := &User{}

	query := `
		UPDATE users
		SET username = $2, email = $3, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, username, email, password_hash, created_at, updated_at, deleted_at
	`

	// Executes the query and maps the result to the user. If everything's ok returns it
	err := ur.db.QueryRowContext(ctx, query, userID, username, email).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.DeletedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}

		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	return user, nil
}

// Delete performs a soft delete by setting the deleted_at timestamp
func (ur *UsersRepository) Delete(ctx context.Context, userID string) error {
	// Creates the query
	query := `
		UPDATE users
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`

	// Executes the query
	result, err := ur.db.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	// Checks if any user has been deleted
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrUserNotFound
	}

	return nil
}
