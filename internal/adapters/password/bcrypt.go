// Package password hashes and verifies user passwords with bcrypt.
package password

import "golang.org/x/crypto/bcrypt"

// Bcrypt implements platform.PasswordHasher.
type Bcrypt struct{ Cost int }

func (b Bcrypt) Hash(plain string) (string, error) {
	cost := b.Cost
	if cost == 0 {
		cost = bcrypt.DefaultCost
	}
	h, err := bcrypt.GenerateFromPassword([]byte(plain), cost)
	return string(h), err
}

func (Bcrypt) Verify(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
