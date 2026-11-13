//go:build darwin

package librarymonitor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsevents"

	"github.com/Silo-Server/silo-server/internal/fspath"
)

func testBackend(t *testing.T) *fseventsBackend {
	t.Helper()
	b, err := newFSEventsBackend(BackendOptions{})
	if err != nil {
		t.Fatalf("newFSEventsBackend() error = %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b.(*fseventsBackend)
}

// FSEvents reports that a path changed, not what happened to it, so translate
// decides the kind from the flags and whether the path exists. Each case here
// is a kind the monitor acts on differently: a wrong mapping either rescans
// nothing or marks a present file missing.
func TestTranslateMapsFlagsToEventKinds(t *testing.T) {
	b := testBackend(t)
	root := t.TempDir()
	encodedRoot := fspath.Encode(root)

	present := filepath.Join(root, "Film (1999).mkv")
	if err := os.WriteFile(present, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	presentDir := filepath.Join(root, "Season 01")
	if err := os.Mkdir(presentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(root, "gone.mkv")

	for _, tc := range []struct {
		name     string
		event    fsevents.Event
		wantSend bool
		wantKind EventKind
		wantDir  bool
	}{
		{
			name:     "root changed is a lost root",
			event:    fsevents.Event{Path: root, Flags: fsevents.RootChanged},
			wantSend: true, wantKind: EventRootLost,
		},
		{
			name:     "must scan subdirs is an overflow",
			event:    fsevents.Event{Path: present, Flags: fsevents.MustScanSubDirs},
			wantSend: true, wantKind: EventOverflow,
		},
		{
			name:     "kernel dropped is an overflow",
			event:    fsevents.Event{Path: present, Flags: fsevents.KernelDropped},
			wantSend: true, wantKind: EventOverflow,
		},
		{
			name:     "a vanished path is a delete",
			event:    fsevents.Event{Path: missing, Flags: fsevents.ItemRemoved | fsevents.ItemIsFile},
			wantSend: true, wantKind: EventDelete,
		},
		{
			name:     "a created file is a create",
			event:    fsevents.Event{Path: present, Flags: fsevents.ItemCreated | fsevents.ItemIsFile},
			wantSend: true, wantKind: EventCreate,
		},
		{
			name:     "a created directory is a create marked as a directory",
			event:    fsevents.Event{Path: presentDir, Flags: fsevents.ItemCreated | fsevents.ItemIsDir},
			wantSend: true, wantKind: EventCreate, wantDir: true,
		},
		{
			name:     "a renamed path that now exists arrived here",
			event:    fsevents.Event{Path: present, Flags: fsevents.ItemRenamed | fsevents.ItemIsFile},
			wantSend: true, wantKind: EventMovedTo,
		},
		{
			name:     "a modified file that still exists settled",
			event:    fsevents.Event{Path: present, Flags: fsevents.ItemModified | fsevents.ItemIsFile},
			wantSend: true, wantKind: EventCloseWrite,
		},
		{
			name:     "a directory whose contents changed is left to its children",
			event:    fsevents.Event{Path: presentDir, Flags: fsevents.ItemModified | fsevents.ItemIsDir},
			wantSend: false,
		},
		{
			name:     "the root itself is not an entry in itself",
			event:    fsevents.Event{Path: root, Flags: fsevents.ItemModified | fsevents.ItemIsDir},
			wantSend: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ev, send := b.translate(encodedRoot, root, tc.event)
			if send != tc.wantSend {
				t.Fatalf("send = %v, want %v (event %+v)", send, tc.wantSend, ev)
			}
			if !send {
				return
			}
			if ev.Kind != tc.wantKind {
				t.Errorf("kind = %v, want %v", ev.Kind, tc.wantKind)
			}
			if ev.IsDir != tc.wantDir {
				t.Errorf("IsDir = %v, want %v", ev.IsDir, tc.wantDir)
			}
		})
	}
}

// The monitor matches Dir against the paths it recorded, which are
// fspath-encoded and carry no trailing slash. A split that leaves one makes
// every event miss.
func TestTranslateSplitsPathForTheMonitor(t *testing.T) {
	b := testBackend(t)
	root := t.TempDir()
	file := filepath.Join(root, "Show", "Season 01", "ep.mkv")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}

	ev, send := b.translate(fspath.Encode(root), root, fsevents.Event{
		Path:  file,
		Flags: fsevents.ItemCreated | fsevents.ItemIsFile,
	})
	if !send {
		t.Fatal("translate() dropped a created file")
	}
	if want := fspath.Encode(filepath.Dir(file)); ev.Dir != want {
		t.Errorf("Dir = %q, want %q", ev.Dir, want)
	}
	if ev.Name != "ep.mkv" {
		t.Errorf("Name = %q, want ep.mkv", ev.Name)
	}
}

// FSEvents paths arrive without a leading slash; a path left relative would
// never match a recorded directory.
func TestTranslateRestoresAbsolutePaths(t *testing.T) {
	b := testBackend(t)
	root := t.TempDir()
	file := filepath.Join(root, "Film.mkv")
	if err := os.WriteFile(file, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}

	ev, send := b.translate(fspath.Encode(root), root, fsevents.Event{
		Path:  file[1:], // as FSEvents delivers it
		Flags: fsevents.ItemCreated | fsevents.ItemIsFile,
	})
	if !send {
		t.Fatal("translate() dropped a created file")
	}
	if want := fspath.Encode(root); ev.Dir != want {
		t.Errorf("Dir = %q, want %q", ev.Dir, want)
	}
}
