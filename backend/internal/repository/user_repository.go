package repository

import (
	"context"

	"github.com/example/invest-portfolio-platform/backend/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserRepository struct{ db *pgxpool.Pool }

func NewUserRepository(db *pgxpool.Pool) *UserRepository { return &UserRepository{db: db} }

func (r *UserRepository) Create(ctx context.Context, email string, passwordHash string, displayName string) (*models.User, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, display_name, role)
		VALUES ($1, $2, $3, 'user')
		RETURNING id, email, password_hash, display_name, role, is_active, created_at, updated_at
	`, email, passwordHash, displayName)
	var user models.User
	err := row.Scan(&user.ID, &user.Email, &user.PasswordHash, &user.DisplayName, &user.Role, &user.IsActive, &user.CreatedAt, &user.UpdatedAt)
	return &user, err
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, email, password_hash, display_name, role, is_active, created_at, updated_at
		FROM users WHERE email = $1 AND is_active = true
	`, email)
	var user models.User
	err := row.Scan(&user.ID, &user.Email, &user.PasswordHash, &user.DisplayName, &user.Role, &user.IsActive, &user.CreatedAt, &user.UpdatedAt)
	return &user, err
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (*models.User, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, email, password_hash, display_name, role, is_active, created_at, updated_at
		FROM users WHERE id = $1 AND is_active = true
	`, id)
	var user models.User
	err := row.Scan(&user.ID, &user.Email, &user.PasswordHash, &user.DisplayName, &user.Role, &user.IsActive, &user.CreatedAt, &user.UpdatedAt)
	return &user, err
}
