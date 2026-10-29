package auth

import (
	"context"
	"strings"
	"testing"
)

// cheap parameters: the tests check behaviour, not cost.
var testParams = Params{Time: 1, MemoryKiB: 64, Threads: 1}

func TestHasher_HashVerifyRoundTrip(t *testing.T) {
	h := NewHasher(testParams)
	enc, err := h.Hash("a long enough passphrase")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(enc, "$argon2id$v=19$m=64,t=1,p=1$") {
		t.Errorf("encoding %q does not carry its parameters", enc)
	}
	if strings.Contains(enc, "passphrase") {
		t.Error("the encoding contains the secret")
	}
	ok, err := h.Verify(context.Background(), enc, "a long enough passphrase")
	if err != nil || !ok {
		t.Fatalf("right passphrase: %v, %v", ok, err)
	}
	if ok, _ := h.Verify(context.Background(), enc, "a long enough passphrasE"); ok {
		t.Error("a wrong passphrase verified")
	}
}

func TestHasher_TwoHashesOfTheSamePassphraseDiffer(t *testing.T) {
	h := NewHasher(testParams)
	a, _ := h.Hash("same passphrase here")
	b, _ := h.Hash("same passphrase here")
	if a == b {
		t.Error("the salt is not random")
	}
}

func TestHasher_VerifyReadsTheParametersFromTheEncoding(t *testing.T) {
	enc, _ := NewHasher(testParams).Hash("another long passphrase")
	// A hasher with different settings still verifies an older hash, so the cost can be raised later.
	other := NewHasher(Params{Time: 2, MemoryKiB: 128, Threads: 1})
	if ok, err := other.Verify(context.Background(), enc, "another long passphrase"); err != nil || !ok {
		t.Fatalf("older hash: %v, %v", ok, err)
	}
}

func TestHasher_RejectsAMalformedEncoding(t *testing.T) {
	h := NewHasher(testParams)
	for _, bad := range []string{"", "plain", "$argon2id$v=19$m=x,t=1,p=1$AAAA$AAAA", "$argon2i$v=19$m=64,t=1,p=1$AAAA$AAAA", "$argon2id$v=19$m=64,t=1,p=1$!!$!!"} {
		if ok, err := h.Verify(context.Background(), bad, "x"); ok || err == nil {
			t.Errorf("%q: ok=%v err=%v, want a refusal", bad, ok, err)
		}
	}
}

func TestHasher_VerifyHonoursACancelledContext(t *testing.T) {
	h := NewHasher(testParams)
	enc, _ := h.Hash("a long enough passphrase")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.Verify(ctx, enc, "a long enough passphrase"); err == nil {
		t.Error("a cancelled request still ran the hash")
	}
}

func TestHasher_BoundsConcurrentVerifications(t *testing.T) {
	h := NewHasher(testParams)
	if cap(h.sem) != maxConcurrentHashes {
		t.Errorf("concurrency bound = %d, want %d", cap(h.sem), maxConcurrentHashes)
	}
}

func TestCheckStrength(t *testing.T) {
	good := []string{"correct horse battery", "twelve chars!", "a few words is fine"}
	for _, p := range good {
		if err := CheckStrength(p); err != nil {
			t.Errorf("%q refused: %v", p, err)
		}
	}
	bad := map[string]string{
		"short":            "at least 12",
		"aaaaaaaaaaaa":     "one character",
		"password1234":     "too common",
		"Password1234":     "too common",
		"            ":     "blank",
		"123456789012":     "too common",
		"qwertyuiop12":     "too common",
		"llm-proxy-admin1": "",
	}
	for p, want := range bad {
		err := CheckStrength(p)
		if want == "" {
			continue // a passphrase that merely looks weak to a person is allowed: no composition rules
		}
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err = %v, want one containing %q", p, err, want)
		}
	}
}
