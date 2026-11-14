package stackport

import (
	"errors"
	"testing"
)

func TestKeyCacheReadsTheKeyOnceUntilDropped(t *testing.T) {
	reads := 0
	c := NewKeyCache(func() ([]byte, error) { reads++; return make([]byte, 32), nil })
	for range 5 {
		if k, err := c.Get(); err != nil || len(k) != 32 {
			t.Fatal(k, err)
		}
	}
	if reads != 1 {
		t.Errorf("the key was read %d times; a window of thousands of frames must not read the disk for each", reads)
	}
	c.Drop()
	if _, err := c.Get(); err != nil || reads != 2 {
		t.Errorf("after a drop (factory reset) the next call reads again: reads=%d err=%v", reads, err)
	}
}

func TestKeyCacheDoesNotRememberAFailure(t *testing.T) {
	fail := true
	c := NewKeyCache(func() ([]byte, error) {
		if fail {
			return nil, errors.New("no key yet")
		}
		return make([]byte, 32), nil
	})
	if _, err := c.Get(); err == nil {
		t.Fatal("want the error")
	}
	fail = false
	if _, err := c.Get(); err != nil {
		t.Errorf("a key that appears later must be found: %v", err)
	}
}

func TestDroppedKeyIsZeroed(t *testing.T) {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i + 1)
	}
	c := NewKeyCache(func() ([]byte, error) { return k, nil })
	held, _ := c.Get()
	c.Drop()
	for _, b := range held {
		if b != 0 {
			t.Fatal("a dropped key stays in memory")
		}
	}
}
