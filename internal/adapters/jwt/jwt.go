// Package jwt issues and verifies HS256 access tokens carrying an
// identity.Principal.
package jwt

import (
	"errors"
	"fmt"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"

	"github.com/OubeidHanini/plantorq/internal/domain/identity"
)

const issuer = "plantorq"

type claims struct {
	identity.Principal
	gojwt.RegisteredClaims
}

// Tokens implements platform.TokenIssuer and the HTTP layer's verifier.
type Tokens struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func New(secret string, ttl time.Duration) (*Tokens, error) {
	if len(secret) < 32 {
		return nil, errors.New("jwt: secret must be at least 32 bytes")
	}
	return &Tokens{secret: []byte(secret), ttl: ttl, now: time.Now}, nil
}

func (t *Tokens) Issue(p identity.Principal) (string, time.Time, error) {
	now := t.now()
	exp := now.Add(t.ttl)
	c := claims{Principal: p, RegisteredClaims: gojwt.RegisteredClaims{
		Issuer:    issuer,
		Subject:   fmt.Sprint(p.UserID),
		IssuedAt:  gojwt.NewNumericDate(now),
		ExpiresAt: gojwt.NewNumericDate(exp),
	}}
	tok, err := gojwt.NewWithClaims(gojwt.SigningMethodHS256, c).SignedString(t.secret)
	return tok, exp, err
}

func (t *Tokens) Verify(token string) (identity.Principal, error) {
	var c claims
	_, err := gojwt.ParseWithClaims(token, &c,
		func(*gojwt.Token) (any, error) { return t.secret, nil },
		gojwt.WithValidMethods([]string{gojwt.SigningMethodHS256.Alg()}),
		gojwt.WithIssuer(issuer),
		gojwt.WithExpirationRequired(),
		gojwt.WithTimeFunc(t.now),
	)
	if err != nil {
		return identity.Principal{}, err
	}
	if !c.Role.Valid() || c.TenantID == 0 {
		return identity.Principal{}, errors.New("jwt: token is missing tenant or role")
	}
	return c.Principal, nil
}
