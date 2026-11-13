//go:build darwin

package librarymonitor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Silo-Server/silo-server/internal/fspath"
)

// FSEvents coalesces for fseventsLatency before delivering, and the kernel
// adds its own delay, so a real stream needs far longer than a unit test.
const fseventsTestTimeout = 15 * time.Second

// liveBackend starts a backend watching a fresh directory.
func liveBackend(t *testing.T) (*fseventsBackend, string) {
	t.Helper()
	// FSEvents reports resolved paths, and macOS temp dirs are under a
	// symlinked /var, so resolve up front or no event will ever match.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b, err := newFSEventsBackend(BackendOptions{Logger: quietLogger()})
	if err != nil {
		t.Fatalf("newFSEventsBackend() error = %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if err := b.AddRoot(context.Background(), fspath.Encode(root)); err != nil {
		t.Fatalf("AddRoot() error = %v", err)
	}
	return b.(*fseventsBackend), root
}

// awaitEvent waits for an event whose path matches want, returning it. It
// drains unrelated events rather than failing on them: FSEvents reports
// directory changes alongside the file ones.
func awaitEvent(t *testing.T, b *fseventsBackend, wantPath string, kinds ...EventKind) Event {
	t.Helper()
	want := fspath.Encode(wantPath)
	deadline := time.After(fseventsTestTimeout)
	for {
		select {
		case ev, ok := <-b.Events():
			if !ok {
				t.Fatal("event channel closed while waiting")
			}
			got := ev.Dir
			if ev.Name != "" {
				got = filepath.Join(ev.Dir, ev.Name)
			}
			if got != want {
				continue
			}
			if len(kinds) == 0 {
				return ev
			}
			for _, k := range kinds {
				if ev.Kind == k {
					return ev
				}
			}
			t.Logf("ignoring %v for %s while waiting for %v", ev.Kind, got, kinds)
		case <-deadline:
			t.Fatalf("no %v event for %s within %s", kinds, want, fseventsTestTimeout)
		}
	}
}

func TestFSEventsReportsCreatedFile(t *testing.T) {
	b, root := liveBackend(t)
	path := filepath.Join(root, "Film (1999).mkv")
	if err := os.WriteFile(path, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	ev := awaitEvent(t, b, path, EventCreate, EventCloseWrite)
	if ev.IsDir {
		t.Errorf("IsDir = true for a file")
	}
}

func TestFSEventsReportsCreatedDirectory(t *testing.T) {
	b, root := liveBackend(t)
	dir := filepath.Join(root, "Show", "Season 01")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	ev := awaitEvent(t, b, dir, EventCreate)
	if !ev.IsDir {
		t.Error("IsDir = false for a directory")
	}
}

// A new file in a directory created after the stream started must still be
// reported: FSEvents is recursive, so no new watch is needed, which is the
// property this backend relies on.
func TestFSEventsReportsFileInNewDirectory(t *testing.T) {
	b, root := liveBackend(t)
	dir := filepath.Join(root, "Added Later")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	awaitEvent(t, b, dir, EventCreate)

	path := filepath.Join(dir, "ep.mkv")
	if err := os.WriteFile(path, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	awaitEvent(t, b, path, EventCreate, EventCloseWrite)
}

func TestFSEventsReportsDeletedFile(t *testing.T) {
	b, root := liveBackend(t)
	path := filepath.Join(root, "doomed.mkv")
	if err := os.WriteFile(path, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	awaitEvent(t, b, path, EventCreate, EventCloseWrite)

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	awaitEvent(t, b, path, EventDelete)
}

// A rename inside the tree arrives as two independent paths. The old path must
// report as gone and the new one as arrived, or the monitor leaves a stale row.
func TestFSEventsReportsRenameAsBothPaths(t *testing.T) {
	b, root := liveBackend(t)
	from := filepath.Join(root, "old name.mkv")
	to := filepath.Join(root, "new name.mkv")
	if err := os.WriteFile(from, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	awaitEvent(t, b, from, EventCreate, EventCloseWrite)

	if err := os.Rename(from, to); err != nil {
		t.Fatal(err)
	}
	awaitEvent(t, b, from, EventDelete)
	awaitEvent(t, b, to, EventMovedTo, EventCreate, EventCloseWrite)
}

// Directories() is what the monitor uses to see a root was covered.
func TestFSEventsCountsDirectories(t *testing.T) {
	b, root := liveBackend(t)
	if got := b.Directories(fspath.Encode(root)); got < 1 {
		t.Errorf("Directories() = %d, want at least the root itself", got)
	}
	if got := b.Directories("/not/a/root"); got != 0 {
		t.Errorf("Directories(unknown) = %d, want 0", got)
	}
}

// Close must not panic, deadlock, or leave the event channel open while a
// stream is still delivering.
func TestFSEventsCloseDuringActivity(t *testing.T) {
	b, root := liveBackend(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 50 {
			_ = os.WriteFile(filepath.Join(root, "f"+string(rune('a'+i%26))+".mkv"), []byte("x"), 0o600)
		}
	}()
	<-done

	if err := b.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	// A second Close is a no-op, not a double-close panic.
	if err := b.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if _, ok := <-b.Events(); ok {
		t.Error("event channel still open after Close")
	}
}

// RemoveRoot must stop delivery for that root without closing the backend.
func TestFSEventsRemoveRootStopsDelivery(t *testing.T) {
	b, root := liveBackend(t)
	b.RemoveRoot(fspath.Encode(root))
	if got := b.Directories(fspath.Encode(root)); got != 0 {
		t.Errorf("Directories() = %d after RemoveRoot, want 0", got)
	}
	// Removing an unknown root is a no-op.
	b.RemoveRoot("/not/a/root")
}
