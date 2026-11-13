//go:build darwin

package embeddedsub

import (
	"os"
	"path/filepath"
	"testing"
)

// adviseRandom is advice, so it has no observable result; the contract is that
// it never fails the read it advises on. These cover the cases that could
// panic or corrupt the file handle instead.
func TestAdviseRandomKeepsFileUsable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subtitle.mkv")
	want := []byte("subtitle payload")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	adviseRandom(f)

	got := make([]byte, len(want))
	if _, err := f.Read(got); err != nil {
		t.Fatalf("Read() after adviseRandom error = %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("read %q, want %q", got, want)
	}
}

func TestAdviseRandomOnClosedFileDoesNotPanic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "closed.mkv")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	adviseRandom(f) // must be a no-op, not a panic
}
