package platform

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/OubeidHanini/plantorq/internal/domain/identity"
)

type AuthService struct {
	users     UserRepository
	passwords PasswordVerifier
	tokens    TokenIssuer
}

func NewAuthService(users UserRepository, passwords PasswordVerifier, tokens TokenIssuer) *AuthService {
	return &AuthService{users: users, passwords: passwords, tokens: tokens}
}

type Session struct {
	Token     string             `json:"token"`
	ExpiresAt time.Time          `json:"expires_at"`
	User      identity.Principal `json:"user"`
}

// Login checks credentials and returns a signed session token. Unknown users
// and wrong passwords yield the same error, so the endpoint cannot be used to
// discover which e-mail addresses exist.
func (s *AuthService) Login(ctx context.Context, email, password string) (Session, error) {
	u, err := s.users.UserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if errors.Is(err, ErrNotFound) {
		return Session{}, ErrUnauthorized
	}
	if err != nil {
		return Session{}, err
	}
	if !s.passwords.Verify(u.PasswordHash, password) {
		return Session{}, ErrUnauthorized
	}
	p := u.Principal()
	tok, exp, err := s.tokens.Issue(p)
	if err != nil {
		return Session{}, err
	}
	return Session{Token: tok, ExpiresAt: exp, User: p}, nil
}
