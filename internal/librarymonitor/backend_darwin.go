//go:build darwin

package librarymonitor

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsevents"

	"github.com/Silo-Server/silo-server/internal/fspath"
)

// BackendFSEvents is the backend name reported in the status on macOS.
const BackendFSEvents = "fsevents"

// fseventsBackend watches roots with FSEvents.
//
// FSEvents differs from inotify in two ways that shape this file. It is
// recursive, so one stream covers a root and every directory below it and the
// per-directory bookkeeping inotify needs does not exist: AddRoot starts a
// stream, Directories reports the walked count only so the monitor's progress
// checks still work, and no watch limit can be reached. And it reports that a
// path changed rather than which operation changed it, so creates, writes and
// deletes are distinguished by stat'ing the path when the event arrives.
type fseventsBackend struct {
	logger *slog.Logger
	events chan Event

	mu     sync.Mutex
	roots  map[string]*fseventsRoot
	closed bool

	wg sync.WaitGroup
}

type fseventsRoot struct {
	stream *fsevents.EventStream
	dirs   int
	done   chan struct{}
}

// newInotifyBackend is the construction seam monitor.go calls. On darwin it
// builds the FSEvents backend; the inotify hooks are a Linux test seam and are
// ignored here.
func newInotifyBackend(opts BackendOptions, _ inotifyHooks) (Backend, error) {
	return newFSEventsBackend(opts)
}

func newFSEventsBackend(opts BackendOptions) (Backend, error) {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &fseventsBackend{
		logger: logger,
		events: make(chan Event, 1024),
		roots:  map[string]*fseventsRoot{},
	}, nil
}

func (b *fseventsBackend) Name() string { return BackendFSEvents }

// AddRoot starts a recursive stream for root. It walks the tree first so
// Directories reports a real count and the monitor sees the same directory set
// the scanner would enter; the stream itself needs no per-directory state.
func (b *fseventsBackend) AddRoot(ctx context.Context, root string) error {
	decoded := fspath.Decode(root)
	info, err := os.Stat(decoded)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("root is not a directory")
	}

	// The stream is recursive, so the walk only counts the directories the
	// scanner would enter. Directories() reports that count, which the
	// monitor uses to see that a root was covered.
	dirs := 0
	visitor := walkVisitor{
		enter: func(string, bool) (bool, error) {
			dirs++
			return true, nil
		},
		listed: func(_ string, skipped bool) bool { return skipped },
	}
	if err := walkTree(ctx, root, visitor); err != nil && !errors.Is(err, context.Canceled) {
		b.logger.Warn("fsevents: walking root failed; monitoring it anyway", "root", decoded, "error", err)
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return errRootReleased
	}
	if existing, ok := b.roots[root]; ok {
		// A re-walk after an overflow: the stream already covers everything.
		existing.dirs = dirs
		return nil
	}

	stream := &fsevents.EventStream{
		Paths:   []string{decoded},
		Latency: fseventsLatency,
		Flags:   fsevents.FileEvents | fsevents.WatchRoot,
	}
	stream.Start()
	r := &fseventsRoot{stream: stream, dirs: dirs, done: make(chan struct{})}
	b.roots[root] = r

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		b.consume(root, decoded, r)
	}()
	return nil
}

func (b *fseventsBackend) RemoveRoot(root string) {
	b.mu.Lock()
	r, ok := b.roots[root]
	if ok {
		delete(b.roots, root)
	}
	b.mu.Unlock()
	if ok {
		r.stream.Stop()
		close(r.done)
	}
}

func (b *fseventsBackend) Directories(root string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	if r, ok := b.roots[root]; ok {
		return r.dirs
	}
	return 0
}

func (b *fseventsBackend) Events() <-chan Event { return b.events }

func (b *fseventsBackend) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	roots := make([]*fseventsRoot, 0, len(b.roots))
	for _, r := range b.roots {
		roots = append(roots, r)
	}
	b.roots = map[string]*fseventsRoot{}
	b.mu.Unlock()

	for _, r := range roots {
		r.stream.Stop()
		close(r.done)
	}
	b.wg.Wait()
	close(b.events)
	return nil
}

// fseventsLatency is how long FSEvents coalesces before delivering. The
// monitor debounces again, so this only trades promptness for fewer wakeups.
const fseventsLatency = 500 * time.Millisecond

// consume translates one root's FSEvents stream into Events.
//
// FSEvents says a path changed, not what happened to it, so the kind is
// decided by whether the path exists now and what the flags report. The
// monitor treats EventCloseWrite as "this file settled"; FSEvents has no
// close-write notification, so a modified file that still exists is reported
// as one. A file still being copied therefore reports repeatedly until it
// stops changing, which the monitor's debounce absorbs.
func (b *fseventsBackend) consume(root, decodedRoot string, r *fseventsRoot) {
	for {
		select {
		case <-r.done:
			return
		case batch, ok := <-r.stream.Events:
			if !ok {
				return
			}
			for _, e := range batch {
				if ev, send := b.translate(root, decodedRoot, e); send {
					select {
					case b.events <- ev:
					case <-r.done:
						return
					}
				}
			}
		}
	}
}

func (b *fseventsBackend) translate(root, decodedRoot string, e fsevents.Event) (Event, bool) {
	path := e.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// The root itself moved, was deleted, or was unmounted.
	if e.Flags&fsevents.RootChanged != 0 {
		return Event{Kind: EventRootLost, Root: root}, true
	}
	// The kernel dropped events: the monitor re-walks on this.
	if e.Flags&(fsevents.MustScanSubDirs|fsevents.KernelDropped|fsevents.UserDropped) != 0 {
		return Event{Kind: EventOverflow}, true
	}
	if path == decodedRoot {
		return Event{}, false
	}

	encoded := fspath.Encode(path)
	dir, name := filepath.Split(encoded)
	dir = strings.TrimSuffix(dir, "/")

	isDir := e.Flags&fsevents.ItemIsDir != 0
	_, statErr := os.Lstat(path)
	exists := statErr == nil

	switch {
	case !exists:
		// Removed, or renamed away. FSEvents reports a rename as a change on
		// both the old and the new path, so each side is reported on its own:
		// the monitor pairs a delete with a create by identity.
		return Event{Kind: EventDelete, Dir: dir, Name: name, IsDir: isDir}, true
	case e.Flags&fsevents.ItemCreated != 0:
		return Event{Kind: EventCreate, Dir: dir, Name: name, IsDir: isDir}, true
	case e.Flags&fsevents.ItemRenamed != 0:
		// Arrived under this name from somewhere else.
		return Event{Kind: EventMovedTo, Dir: dir, Name: name, IsDir: isDir}, true
	case isDir:
		// A directory whose contents changed; the child's own event covers it.
		return Event{}, false
	default:
		return Event{Kind: EventCloseWrite, Dir: dir, Name: name}, true
	}
}
