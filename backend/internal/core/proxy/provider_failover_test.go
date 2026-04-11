package proxy

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"llm-proxy/internal/core/llm"
)

type mockFailoverSelector struct {
	primary  string
	fallback string
	def      string
}

func (m *mockFailoverSelector) DefaultModel() (string, error) { return m.def, nil }
func (m *mockFailoverSelector) PrimaryModel() string           { return m.primary }
func (m *mockFailoverSelector) FallbackModel() string          { return m.fallback }

type mockRuntime struct {
	llm.RuntimeManager
	instances map[string]llm.ModelInstance
	errors    map[string]error
}

func (m *mockRuntime) GetInstance(ctx context.Context, name string) (llm.ModelInstance, error) {
	if err, ok := m.errors[name]; ok {
		return llm.ModelInstance{}, err
	}
	return m.instances[name], nil
}

func (m *mockRuntime) RecordActivity(name string) {}

func TestRuntimeClientProvider_Failover(t *testing.T) {
	newClient := func(url string, model string, headers http.Header) Client {
		return nil // Client doesn't matter for these tests as we check GetClient result
	}

	t.Run("Should use primary when healthy", func(t *testing.T) {
		sel := &mockFailoverSelector{primary: "primary", fallback: "fallback"}
		run := &mockRuntime{
			instances: map[string]llm.ModelInstance{
				"primary":  {Name: "primary", URL: "http://primary"},
				"fallback": {Name: "fallback", URL: "http://fallback"},
			},
		}
		p := NewRuntimeClientProvider(sel, run, newClient)

		_, err := p.GetClient(context.Background())
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if p.(*RuntimeClientProvider).model != "primary" {
			t.Errorf("expected model primary, got %s", p.(*RuntimeClientProvider).model)
		}
	})

	t.Run("Should NOT fallback when primary is starting", func(t *testing.T) {
		sel := &mockFailoverSelector{primary: "primary", fallback: "fallback"}
		run := &mockRuntime{
			errors: map[string]error{
				"primary": llm.ErrModelStarting,
			},
		}
		p := NewRuntimeClientProvider(sel, run, newClient)

		_, err := p.GetClient(context.Background())
		if !errors.Is(err, llm.ErrModelStarting) {
			t.Errorf("expected ErrModelStarting, got %v", err)
		}
	})

	t.Run("Should fallback when primary has terminal error", func(t *testing.T) {
		sel := &mockFailoverSelector{primary: "primary", fallback: "fallback"}
		run := &mockRuntime{
			instances: map[string]llm.ModelInstance{
				"fallback": {Name: "fallback", URL: "http://fallback"},
			},
			errors: map[string]error{
				"primary": errors.New("terminal failure"),
			},
		}
		p := NewRuntimeClientProvider(sel, run, newClient)

		_, err := p.GetClient(context.Background())
		if err != nil {
			t.Fatalf("expected fallback to succeed, got %v", err)
		}
		if p.(*RuntimeClientProvider).model != "fallback" {
			t.Errorf("expected model fallback, got %s", p.(*RuntimeClientProvider).model)
		}
	})

	t.Run("Should use default when no primary configured", func(t *testing.T) {
		sel := &mockFailoverSelector{def: "default"}
		run := &mockRuntime{
			instances: map[string]llm.ModelInstance{
				"default": {Name: "default", URL: "http://default"},
			},
		}
		p := NewRuntimeClientProvider(sel, run, newClient)

		_, err := p.GetClient(context.Background())
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if p.(*RuntimeClientProvider).model != "default" {
			t.Errorf("expected model default, got %s", p.(*RuntimeClientProvider).model)
		}
	})
}
