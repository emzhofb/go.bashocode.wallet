package repository

import (
	"database/sql"
	"errors"

	"github.com/emzhofb/gowallet/user-service/internal/user/model"
)

type UserRepository interface {
	Create(user *model.User) error
	GetByID(id string) (*model.User, error)
	GetByEmail(email string) (*model.User, error)
}

type mysqlUserRepository struct {
	db *sql.DB
}

func NewMySQLUserRepository(db *sql.DB) UserRepository {
	return &mysqlUserRepository{db: db}
}

func (r *mysqlUserRepository) Create(user *model.User) error {
	query := `INSERT INTO users (id, email, password, name, role, is_verified) VALUES (?, ?, ?, ?, ?, ?)`
	_, err := r.db.Exec(query, user.ID, user.Email, user.Password, user.Name, user.Role, user.IsVerified)
	return err
}

func (r *mysqlUserRepository) GetByID(id string) (*model.User, error) {
	query := `SELECT id, email, password, name, role, is_verified, created_at, updated_at FROM users WHERE id = ? AND deleted_at IS NULL`
	row := r.db.QueryRow(query, id)

	var user model.User
	err := row.Scan(&user.ID, &user.Email, &user.Password, &user.Name, &user.Role, &user.IsVerified, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &user, nil
}

func (r *mysqlUserRepository) GetByEmail(email string) (*model.User, error) {
	query := `SELECT id, email, password, name, role, is_verified, created_at, updated_at FROM users WHERE email = ? AND deleted_at IS NULL`
	row := r.db.QueryRow(query, email)

	var user model.User
	err := row.Scan(&user.ID, &user.Email, &user.Password, &user.Name, &user.Role, &user.IsVerified, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &user, nil
}
