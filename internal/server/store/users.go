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
	ID                  uuid.UUID
	EntraOID            *string // nil for local accounts
	Email               string
	DisplayName         string
	Role                string
	PasswordHash        *string // nil for OIDC-only accounts
	ForcePasswordChange bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// userColumns is the standard SELECT list for the users table.
const userColumns = `id, entra_oid, email, display_name, role, password_hash, force_password_change, created_at, updated_at`

// scanUser scans a row into a User.
func scanUser(row pgx.Row) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.EntraOID, &u.Email, &u.DisplayName, &u.Role,
		&u.PasswordHash, &u.ForcePasswordChange, &u.CreatedAt, &u.UpdatedAt)
	return &u, err
}

// CreateUserParams holds the parameters for creating a user (OIDC flow).
type CreateUserParams struct {
	EntraOID    string
	Email       string
	DisplayName string
}

// CreateUser inserts a new OIDC user and returns it.
func (s *Store) CreateUser(ctx context.Context, p CreateUserParams) (*User, error) {
	u, err := scanUser(s.pool.QueryRow(ctx,
		`INSERT INTO users (entra_oid, email, display_name)
		 VALUES ($1, $2, $3)
		 RETURNING `+userColumns,
		p.EntraOID, p.Email, p.DisplayName,
	))
	if err != nil {
		return nil, fmt.Errorf("creating user: %w", err)
	}
	return u, nil
}

// CreateLocalUserParams holds parameters for creating a local (password) user.
type CreateLocalUserParams struct {
	Email               string
	DisplayName         string
	PasswordHash        string
	Role                string
	ForcePasswordChange bool
}

// CreateLocalUser inserts a local user with a password hash and returns it.
func (s *Store) CreateLocalUser(ctx context.Context, p CreateLocalUserParams) (*User, error) {
	u, err := scanUser(s.pool.QueryRow(ctx,
		`INSERT INTO users (entra_oid, email, display_name, password_hash, role, force_password_change)
		 VALUES (NULL, $1, $2, $3, $4, $5)
		 RETURNING `+userColumns,
		p.Email, p.DisplayName, p.PasswordHash, p.Role, p.ForcePasswordChange,
	))
	if err != nil {
		return nil, fmt.Errorf("creating local user: %w", err)
	}
	return u, nil
}

// GetUserByEntraOID looks up a user by their Entra Object ID.
func (s *Store) GetUserByEntraOID(ctx context.Context, oid string) (*User, error) {
	u, err := scanUser(s.pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE entra_oid = $1`, oid,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting user by entra_oid: %w", err)
	}
	return u, nil
}

// GetUserByID looks up a user by their UUID.
func (s *Store) GetUserByID(ctx context.Context, id uuid.UUID) (*User, error) {
	u, err := scanUser(s.pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE id = $1`, id,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting user by id: %w", err)
	}
	return u, nil
}

// GetUserByEmail looks up a user by email address.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	u, err := scanUser(s.pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE email = $1`, email,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting user by email: %w", err)
	}
	return u, nil
}

// CountUsers returns the total number of users.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting users: %w", err)
	}
	return count, nil
}

// UpdateUserPassword sets a new password hash and optionally clears the force-change flag.
func (s *Store) UpdateUserPassword(ctx context.Context, id uuid.UUID, hash string) (*User, error) {
	u, err := scanUser(s.pool.QueryRow(ctx,
		`UPDATE users SET password_hash = $1, force_password_change = false, updated_at = now()
		 WHERE id = $2
		 RETURNING `+userColumns,
		hash, id,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("updating user password: %w", err)
	}
	return u, nil
}

// UpdateUserRole changes a user's role and returns the updated user.
func (s *Store) UpdateUserRole(ctx context.Context, id uuid.UUID, role string) (*User, error) {
	u, err := scanUser(s.pool.QueryRow(ctx,
		`UPDATE users SET role = $1, updated_at = now()
		 WHERE id = $2
		 RETURNING `+userColumns,
		role, id,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("updating user role: %w", err)
	}
	return u, nil
}

// UserListFilters holds optional pagination for listing users.
type UserListFilters struct {
	Limit  int
	Offset int
}

// ListUsers returns users ordered by display name, with pagination.
func (s *Store) ListUsers(ctx context.Context, opts ...UserListFilters) ([]User, error) {
	limit := 100
	offset := 0
	if len(opts) > 0 {
		if opts[0].Limit > 0 {
			limit = opts[0].Limit
		}
		offset = opts[0].Offset
	}

	rows, err := s.pool.Query(ctx,
		`SELECT `+userColumns+` FROM users ORDER BY display_name LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.EntraOID, &u.Email, &u.DisplayName, &u.Role,
			&u.PasswordHash, &u.ForcePasswordChange, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning user: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}
