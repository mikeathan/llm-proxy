package auth

import (
	"context"
	"slices"
	"testing"
)

func TestService_ALockoutAndASignInFromANewAddressAreTold(t *testing.T) {
	s, _, _ := newTestService(t)
	var told []SignInAlert
	s.WithAlerts(func(a SignInAlert) { told = append(told, a) })
	setUp(t, s) // the very first sign-in has nothing to compare with: it is not news
	if len(told) != 0 {
		t.Fatalf("the first sign-in raised %v", told)
	}
	ctx := context.Background()
	if _, err := s.Login(ctx, goodPass, "10.0.0.1", "x"); err != nil || len(told) != 0 {
		t.Fatalf("a known address raised %v (%v)", told, err)
	}
	for range maxFailuresPerAddress + 2 {
		_, _ = s.Login(ctx, "wrong passphrase!", "10.0.0.9", "x")
	}
	if !slices.Equal(told, []SignInAlert{SignInLocked}) {
		t.Fatalf("after a lockout told = %v, want one lockout", told)
	}
	if _, err := s.Login(ctx, goodPass, "10.0.0.7", "x"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(told, []SignInAlert{SignInLocked, SignInNewAddress}) {
		t.Fatalf("told = %v, want the new address too", told)
	}
}
