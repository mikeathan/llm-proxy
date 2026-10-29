package auth

import (
	"regexp"
	"strings"
	"testing"
)

func TestNewToken_ShapeAndHash(t *testing.T) {
	plain, hash, err := newToken()
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^lp_[a-z2-7]{52}$`).MatchString(plain) {
		t.Errorf("token %q is not lp_ + 52 base32 characters (32 random bytes)", plain)
	}
	if hash != hashSecret(plain) || strings.Contains(hash, plain) || len(hash) != 64 {
		t.Errorf("hash %q is not the SHA-256 of the token", hash)
	}
	other, _, _ := newToken()
	if other == plain {
		t.Error("tokens repeat")
	}
}

func TestNewSessionID_IsLongAndHashedForStorage(t *testing.T) {
	id, err := newSessionID()
	if err != nil || len(id) < 40 {
		t.Fatalf("id %q, %v", id, err)
	}
	if hashSecret(id) == id {
		t.Error("the stored form must be a hash")
	}
}

func TestNewSetupCode_FormatAndAlphabet(t *testing.T) {
	re := regexp.MustCompile(`^[ABCDEFGHJKMNPQRSTUVWXYZ23456789]{4}-[ABCDEFGHJKMNPQRSTUVWXYZ23456789]{4}$`)
	for i := 0; i < 50; i++ {
		code, err := newSetupCode()
		if err != nil || !re.MatchString(code) {
			t.Fatalf("code %q: %v", code, err)
		}
	}
}

func TestNormaliseSetupCode_ForgivesCaseSpacesAndTheDash(t *testing.T) {
	if got := normaliseSetupCode("  abcd 2345 "); got != "ABCD-2345" {
		t.Errorf("got %q", got)
	}
	if got := normaliseSetupCode("abcd-2345"); got != "ABCD-2345" {
		t.Errorf("got %q", got)
	}
}
