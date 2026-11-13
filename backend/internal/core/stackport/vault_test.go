package stackport

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"llm-proxy-stacks/sdk"
)

func testVault() Vault {
	key := bytes.Repeat([]byte{7}, 32)
	return Vault{Key: func() ([]byte, error) { return key, nil }}
}

func TestVaultTagIsStableKeyedAndPerPurpose(t *testing.T) {
	v, ctx := testVault(), context.Background()
	a, err := v.Tag(ctx, "radio_other", []byte("aa:bb:cc:dd:ee:ff"))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := v.Tag(ctx, "radio_other", []byte("aa:bb:cc:dd:ee:ff"))
	if !bytes.Equal(a, b) {
		t.Error("the same purpose and value must give the same tag")
	}
	c, _ := v.Tag(ctx, "radio_mine", []byte("aa:bb:cc:dd:ee:ff"))
	if bytes.Equal(a, c) {
		t.Error("two purposes must not give matching tags")
	}
	if bytes.Contains(a, []byte("aa:bb")) || len(a) != 32 {
		t.Errorf("tag must not contain the value: %x", a)
	}
	other := Vault{Key: func() ([]byte, error) { return bytes.Repeat([]byte{8}, 32), nil }}
	if d, _ := other.Tag(ctx, "radio_other", []byte("aa:bb:cc:dd:ee:ff")); bytes.Equal(a, d) {
		t.Error("a different master key must give a different tag")
	}
}

func TestVaultSealOpenRoundTrip(t *testing.T) {
	v, ctx := testVault(), context.Background()
	s1, err := v.Seal(ctx, "radio_mine", []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	s2, _ := v.Seal(ctx, "radio_mine", []byte("secret"))
	if bytes.Equal(s1, s2) || bytes.Contains(s1, []byte("secret")) {
		t.Error("a sealed value must differ every call and hide the value")
	}
	got, err := v.Open(ctx, "radio_mine", s1)
	if err != nil || string(got) != "secret" {
		t.Fatalf("open: %q %v", got, err)
	}
}

func TestVaultOpenRefusesAlteredOrOtherPurpose(t *testing.T) {
	v, ctx := testVault(), context.Background()
	s, _ := v.Seal(ctx, "radio_mine", []byte("secret"))
	if _, err := v.Open(ctx, "radio_other", s); stacksdk.CodeOf(err) != stacksdk.CodeDenied {
		t.Errorf("other purpose: %v", err)
	}
	s[0] ^= 1
	if _, err := v.Open(ctx, "radio_mine", s); stacksdk.CodeOf(err) != stacksdk.CodeDenied {
		t.Errorf("altered: %v", err)
	}
	if _, err := v.Open(ctx, "radio_mine", []byte("garbage")); stacksdk.CodeOf(err) != stacksdk.CodeDenied {
		t.Errorf("garbage: %v", err)
	}
}

func TestVaultRefusesBadInput(t *testing.T) {
	v, ctx := testVault(), context.Background()
	for _, p := range []string{"", "Upper", "has space", "../x", strings.Repeat("a", 33), "1abc"} {
		if _, err := v.Tag(ctx, p, []byte("x")); stacksdk.CodeOf(err) != stacksdk.CodeInvalid {
			t.Errorf("purpose %q: %v", p, err)
		}
	}
	if _, err := v.Tag(ctx, "ok", nil); stacksdk.CodeOf(err) != stacksdk.CodeInvalid {
		t.Errorf("empty value: %v", err)
	}
	if _, err := v.Seal(ctx, "ok", make([]byte, stacksdk.MaxVaultBytes+1)); stacksdk.CodeOf(err) != stacksdk.CodeInvalid {
		t.Errorf("large value: %v", err)
	}
}

func TestVaultUnavailableWithoutAKey(t *testing.T) {
	ctx := context.Background()
	for _, k := range []func() ([]byte, error){
		func() ([]byte, error) { return nil, errors.New("no key") },
		func() ([]byte, error) { return []byte("short"), nil },
	} {
		if _, err := (Vault{Key: k}).Tag(ctx, "ok", []byte("x")); stacksdk.CodeOf(err) != stacksdk.CodeUnavailable {
			t.Errorf("want unavailable, got %v", err)
		}
	}
}
