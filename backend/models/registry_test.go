package models

import (
	"errors"
	"testing"
)

func TestClearDanglingModelRefs(t *testing.T) {
	tests := []struct {
		name         string
		reg          RegistryData
		wantPrimary  string
		wantFallback string
	}{
		{
			name: "keeps refs that exist",
			reg: RegistryData{
				Catalogue:     []ModelRegistryEntry{{Name: "alpha"}, {Name: "beta"}},
				PrimaryModel:  "alpha",
				FallbackModel: "beta",
			},
			wantPrimary:  "alpha",
			wantFallback: "beta",
		},
		{
			name: "clears dangling primary",
			reg: RegistryData{
				Catalogue:     []ModelRegistryEntry{{Name: "beta"}},
				PrimaryModel:  "ghost",
				FallbackModel: "beta",
			},
			wantPrimary:  "",
			wantFallback: "beta",
		},
		{
			name: "clears dangling fallback",
			reg: RegistryData{
				Catalogue:     []ModelRegistryEntry{{Name: "alpha"}},
				PrimaryModel:  "alpha",
				FallbackModel: "ghost",
			},
			wantPrimary:  "alpha",
			wantFallback: "",
		},
		{
			name: "clears both when catalogue is empty",
			reg: RegistryData{
				Catalogue:     nil,
				PrimaryModel:  "alpha",
				FallbackModel: "beta",
			},
			wantPrimary:  "",
			wantFallback: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ClearDanglingModelRefs(&tc.reg)
			if tc.reg.PrimaryModel != tc.wantPrimary {
				t.Fatalf("primary = %q, want %q", tc.reg.PrimaryModel, tc.wantPrimary)
			}
			if tc.reg.FallbackModel != tc.wantFallback {
				t.Fatalf("fallback = %q, want %q", tc.reg.FallbackModel, tc.wantFallback)
			}
		})
	}
}

func TestModelExists(t *testing.T) {
	reg := RegistryData{Catalogue: []ModelRegistryEntry{{Name: "alpha"}}}
	if !ModelExists(&reg, "alpha") {
		t.Fatal("expected alpha to exist")
	}
	if ModelExists(&reg, "ghost") {
		t.Fatal("expected ghost to not exist")
	}
}

func TestModelNotFoundError_As(t *testing.T) {
	err := &ModelNotFoundError{Role: "primary", ModelName: "ghost"}
	var target *ModelNotFoundError
	if !errors.As(err, &target) {
		t.Fatal("expected errors.As to match")
	}
	if target.Error() != `primary model "ghost" does not exist` {
		t.Fatalf("unexpected message: %q", target.Error())
	}
}
