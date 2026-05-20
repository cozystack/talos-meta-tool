package main

import (
	"os"
	"testing"
)

// newTestFile creates a zeroed temp file of 2*Length bytes (two ADV blocks).
func newTestFile(t *testing.T) *os.File {
	t.Helper()

	f, err := os.CreateTemp("", "adv-*.bin")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { os.Remove(f.Name()) })

	if err := f.Truncate(2 * Length); err != nil {
		t.Fatal(err)
	}

	return f
}
