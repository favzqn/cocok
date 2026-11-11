package auth

import (
	"strings"
	"testing"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !VerifyPassword(hash, "correct horse battery staple") {
		t.Fatal("expected password to verify")
	}
	if VerifyPassword(hash, "wrong password") {
		t.Fatal("expected wrong password to fail")
	}
}

func TestPasswordHashUniqueSalts(t *testing.T) {
	h1, _ := HashPassword("same-password")
	h2, _ := HashPassword("same-password")
	if h1 == h2 {
		t.Fatal("expected different hashes for same password (salt must differ)")
	}
}

func TestVerifyRejectsMalformed(t *testing.T) {
	cases := []string{"", "plaintext", "argon2id$v=1$bad", "bcrypt$2b$abc"}
	for _, c := range cases {
		if VerifyPassword(c, "x") {
			t.Errorf("expected reject for %q", c)
		}
	}
}

func TestArgonParamsParsedFromHash(t *testing.T) {
	hash, err := HashPassword("params-check")
	if err != nil {
		t.Fatal(err)
	}
	// format: argon2id$v=1$m=65536,t=1,p=1$salt$hash → 5 parts
	if got := len(strings.Split(hash, "$")); got != 5 {
		t.Fatalf("expected 5 segments, got %d: %s", got, hash)
	}
}
