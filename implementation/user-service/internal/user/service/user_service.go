package service

import (
	pkgerrors "github.com/emzhofb/gowallet/pkg/errors"
	"github.com/emzhofb/gowallet/user-service/internal/user/model"
	"github.com/emzhofb/gowallet/user-service/internal/user/repository"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type UserService interface {
	Register(name, email, password string) (*model.User, error)
	GetProfile(id string) (*model.User, error)
	GetByEmail(email string) (*model.User, error)
}

type userService struct {
	repo repository.UserRepository
}

func NewUserService(repo repository.UserRepository) UserService {
	return &userService{repo: repo}
}

func (s *userService) Register(name, email, password string) (*model.User, error) {
	existing, err := s.repo.GetByEmail(email)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, pkgerrors.ErrDuplicateEntry
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &model.User{
		ID:         uuid.New().String(),
		Email:      email,
		Password:   string(hashedPassword),
		Name:       name,
		Role:       "user",
		IsVerified: true, // Auto-verify for simplicity
	}

	err = s.repo.Create(user)
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (s *userService) GetProfile(id string) (*model.User, error) {
	user, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, pkgerrors.ErrNotFound
	}
	return user, nil
}

func (s *userService) GetByEmail(email string) (*model.User, error) {
	user, err := s.repo.GetByEmail(email)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, pkgerrors.ErrNotFound
	}
	return user, nil
}
