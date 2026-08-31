package models

import (
	"net"
	"testing"
)

// localClassifier returns a classifier with a model host and one local
// interface IP (192.168.50.251) — the shape used at the runtime boundary.
func localClassifier() WorkloadClassifier {
	return NewWorkloadClassifier("0.0.0.0", []net.IP{net.ParseIP("192.168.50.251")})
}

func TestClassifyClient(t *testing.T) {
	c := localClassifier()

	tests := []struct {
		name    string
		baseURL string
		modelID string
		want    bool
	}{
		{
			name:    "loopback endpoint",
			baseURL: "http://127.0.0.1:8081/v1",
			modelID: "some-model",
			want:    true,
		},
		{
			name:    "local interface IP endpoint",
			baseURL: "http://192.168.50.251:8081/v1",
			modelID: "some-model",
			want:    true,
		},
		{
			name:    "remote endpoint with non-gguf model",
			baseURL: "http://192.168.50.60:8084/v1",
			modelID: "Ornith-1.5-35B",
			want:    false,
		},
		{
			name:    "remote endpoint serving a gguf artifact",
			baseURL: "http://192.168.50.60:8084/v1",
			modelID: "/home/mikeathan/dev/models/Ornith-1.5-35B-Q4_K_M.gguf",
			want:    true,
		},
		{
			name:    "remote endpoint with bare gguf model name",
			baseURL: "http://192.168.50.60:8084/v1",
			modelID: "Ornith-1.5-35B-Q4_K_M.gguf",
			want:    true,
		},
		{
			name:    "gguf artifact alone without endpoint",
			baseURL: "",
			modelID: "/models/Qwen3.6-35B-A3B-UD-Q4_K_M.gguf",
			want:    true,
		},
		{
			name:    "empty inputs",
			baseURL: "",
			modelID: "",
			want:    false,
		},
		{
			name:    "case-insensitive gguf suffix",
			baseURL: "http://192.168.50.60:8084/v1",
			modelID: "/models/model.GGUF",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.ClassifyClient(tt.baseURL, tt.modelID); got != tt.want {
				t.Errorf("ClassifyClient(%q, %q) = %v, want %v", tt.baseURL, tt.modelID, got, tt.want)
			}
		})
	}
}
