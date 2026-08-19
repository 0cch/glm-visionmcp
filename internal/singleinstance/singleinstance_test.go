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

func TestLockPathWithPrecedence(t *testing.T) {
	// explicit override wins
	got, err := LockPathWith("C:\\locks\\cli.lock", func(string) string { return "C:\\locks\\env.lock" })
	if err != nil {
		t.Fatal(err)
	}
	if got != "C:\\locks\\cli.lock" {
		t.Fatalf("LockPathWith() = %q", got)
	}
	// env var used when no override
	got, err = LockPathWith("", func(string) string { return "C:\\locks\\env.lock" })
	if err != nil {
		t.Fatal(err)
	}
	if got != "C:\\locks\\env.lock" {
		t.Fatalf("LockPathWith() = %q", got)
	}
	// user cache default when neither set
	got, err = LockPathWith("", func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "visionmcp.lock" {
		t.Fatalf("LockPathWith() = %q", got)
	}
}
