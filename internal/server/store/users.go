package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// User represents a row in the users table.
type User struct {
	ID          uuid.UUID
	EntraOID    string
	Email       string
	DisplayName string
	Role        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CreateUserParams holds the parameters for creating a user.
type CreateUserParams struct {
	EntraOID    string
	Email       string
	DisplayName string
}

// CreateUser inserts a new user and returns it.
func (s *Store) CreateUser(ctx context.Context, p CreateUserParams) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`INSERT INTO users (entra_oid, email, display_name)
		 VALUES ($1, $2, $3)
		 RETURNING id, entra_oid, email, display_name, role, created_at, updated_at`,
		p.EntraOID, p.Email, p.DisplayName,
	).Scan(&u.ID, &u.EntraOID, &u.Email, &u.DisplayName, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating user: %w", err)
	}
	return &u, nil
}

// GetUserByEntraOID looks up a user by their Entra Object ID.
func (s *Store) GetUserByEntraOID(ctx context.Context, oid string) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`SELECT id, entra_oid, email, display_name, role, created_at, updated_at
		 FROM users WHERE entra_oid = $1`,
		oid,
	).Scan(&u.ID, &u.EntraOID, &u.Email, &u.DisplayName, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting user by entra_oid: %w", err)
	}
	return &u, nil
}

// GetUserByID looks up a user by their UUID.
func (s *Store) GetUserByID(ctx context.Context, id uuid.UUID) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`SELECT id, entra_oid, email, display_name, role, created_at, updated_at
		 FROM users WHERE id = $1`,
		id,
	).Scan(&u.ID, &u.EntraOID, &u.Email, &u.DisplayName, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting user by id: %w", err)
	}
	return &u, nil
}

// UpdateUserRole changes a user's role and returns the updated user.
func (s *Store) UpdateUserRole(ctx context.Context, id uuid.UUID, role string) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`UPDATE users SET role = $1, updated_at = now()
		 WHERE id = $2
		 RETURNING id, entra_oid, email, display_name, role, created_at, updated_at`,
		role, id,
	).Scan(&u.ID, &u.EntraOID, &u.Email, &u.DisplayName, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("updating user role: %w", err)
	}
	return &u, nil
}

// ListUsers returns all users ordered by display name.
func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, entra_oid, email, display_name, role, created_at, updated_at
		 FROM users ORDER BY display_name`)
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.EntraOID, &u.Email, &u.DisplayName, &u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning user: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}
