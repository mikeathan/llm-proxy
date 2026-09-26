package models

import "testing"

func TestIsResidencyStatus(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{ModelStatusBusy, true},
		{ModelStatusQueued, true},
		{ModelStatusStarting, false},
		{"", false},
		{"BUSY", false},
	}
	for _, tt := range tests {
		if got := IsResidencyStatus(tt.status); got != tt.want {
			t.Errorf("IsResidencyStatus(%q) = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestResidencyAnswer(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantStatus  string
		wantMessage string
	}{
		{
			name:        "busy with message",
			body:        `{"status":"busy","model":"m","message":"serving a","retry_after_seconds":5}`,
			wantStatus:  ModelStatusBusy,
			wantMessage: "serving a",
		},
		{
			name:       "queued without message",
			body:       `{"status":"queued","model":"m"}`,
			wantStatus: ModelStatusQueued,
		},
		{
			name: "starting is not a residency verdict",
			body: `{"status":"starting"}`,
		},
		{
			name: "not json",
			body: "busy",
		},
		{
			name: "openai error shape",
			body: `{"error":{"message":"rate limit exceeded"}}`,
		},
		{
			name: "empty body",
			body: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, message := ResidencyAnswer(tt.body)
			if status != tt.wantStatus || message != tt.wantMessage {
				t.Fatalf("ResidencyAnswer(%q) = (%q, %q), want (%q, %q)",
					tt.body, status, message, tt.wantStatus, tt.wantMessage)
			}
		})
	}
}
