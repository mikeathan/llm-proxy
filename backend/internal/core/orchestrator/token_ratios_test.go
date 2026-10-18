package orchestrator

import (
	"fmt"
	"math"
	"sync"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestTokenRatios(t *testing.T) {
	t.Run("an unknown model gets the conservative default", func(t *testing.T) {
		if got := NewTokenRatios().CharsPerToken("m"); got != DefaultCharsPerToken {
			t.Errorf("got %v, want %v", got, DefaultCharsPerToken)
		}
	})
	t.Run("the first observation is adopted, later ones are blended", func(t *testing.T) {
		r := NewTokenRatios()
		r.Observe("m", 2800, 1000) // 2.8 chars per token
		if got := r.CharsPerToken("m"); got != 2.8 {
			t.Fatalf("first: got %v, want 2.8", got)
		}
		r.Observe("m", 4000, 1000) // 4.0
		if got := r.CharsPerToken("m"); !near(got, 3.4) {
			t.Errorf("blend: got %v, want 3.4", got)
		}
	})
	t.Run("nonsense observations are ignored or clamped", func(t *testing.T) {
		r := NewTokenRatios()
		r.Observe("m", 0, 100)
		r.Observe("m", 100, 0)
		r.Observe("m", -5, 10)
		if got := r.CharsPerToken("m"); got != DefaultCharsPerToken {
			t.Errorf("ignored: got %v", got)
		}
		r.Observe("low", 100, 1000) // 0.1 chars per token
		r.Observe("high", 100000, 10)
		if lo, hi := r.CharsPerToken("low"), r.CharsPerToken("high"); lo != minCharsPerToken || hi != maxCharsPerToken {
			t.Errorf("clamped: got %v and %v", lo, hi)
		}
	})
	t.Run("the number of tracked models is bounded", func(t *testing.T) {
		r := NewTokenRatios()
		for i := range maxTrackedModels + 10 {
			r.Observe(fmt.Sprintf("m%d", i), 3000, 1000)
		}
		if got := r.CharsPerToken(fmt.Sprintf("m%d", maxTrackedModels+5)); got != DefaultCharsPerToken {
			t.Errorf("a model past the cap must not be tracked, got %v", got)
		}
		r.Observe("m0", 4000, 1000) // a tracked model keeps learning
		if got := r.CharsPerToken("m0"); !near(got, 3.5) {
			t.Errorf("tracked model: got %v, want 3.5", got)
		}
	})
	t.Run("a nil store is the default and learns nothing", func(t *testing.T) {
		var r *TokenRatios
		r.Observe("m", 3000, 1000)
		if got := r.CharsPerToken("m"); got != DefaultCharsPerToken {
			t.Errorf("got %v", got)
		}
	})
	t.Run("concurrent use is safe", func(t *testing.T) {
		r := NewTokenRatios()
		var wg sync.WaitGroup
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 200 {
					r.Observe("m", 3000, 1000)
					_ = r.CharsPerToken("m")
				}
			}()
		}
		wg.Wait()
	})
}
