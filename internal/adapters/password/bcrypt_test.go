package password

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHashAndVerify(t *testing.T) {
	b := Bcrypt{Cost: bcrypt.MinCost}
	h, err := b.Hash("s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if !b.Verify(h, "s3cret") || b.Verify(h, "wrong") {
		t.Fatal("bcrypt verification wrong")
	}
}
