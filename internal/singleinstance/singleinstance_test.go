package singleinstance

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestAcquireAllowsOnlyOneOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "visionmcp.lock")
	first, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	defer Release(first)
	if _, err := Acquire(path); !errors.Is(err, ErrLocked) {
		t.Fatalf("second Acquire() error = %v", err)
	}
	Release(first)
	second, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire() after release error = %v", err)
	}
	defer Release(second)
}
