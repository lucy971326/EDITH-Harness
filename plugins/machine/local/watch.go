package machinelocal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"harness/kernel/machine"

	"github.com/fsnotify/fsnotify"
)

const watchDebounce = 200 * time.Millisecond

type localWatch struct {
	owner     *local
	watcher   *fsnotify.Watcher
	target    string
	source    string
	directory bool
	events    chan machine.FileWatchEvent
	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

func (m *local) Watch(path string) (machine.FileWatch, error) {
	target, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("machine-local: watch %q: %w", path, err)
	}
	target = filepath.Clean(target)
	info, err := os.Stat(target)
	if err != nil {
		return nil, fmt.Errorf("machine-local: watch %q: %w", path, err)
	}
	source, err := filepath.EvalSymlinks(target)
	if err != nil {
		return nil, fmt.Errorf("machine-local: resolve watch target %q: %w", path, err)
	}
	source = filepath.Clean(source)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("machine-local: create watcher: %w", err)
	}
	watchPaths := []string{filepath.Dir(target), filepath.Dir(source)}
	if info.IsDir() {
		watchPaths = append(watchPaths, source)
	}
	added := make([]string, 0, len(watchPaths))
	for _, watchPath := range watchPaths {
		duplicate := false
		for _, existing := range added {
			if samePath(existing, watchPath) {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		err = watcher.Add(watchPath)
		if err != nil {
			_ = watcher.Close()
			return nil, fmt.Errorf("machine-local: watch %q: %w", watchPath, err)
		}
		added = append(added, watchPath)
	}

	watch := &localWatch{
		owner:     m,
		watcher:   watcher,
		target:    target,
		source:    source,
		directory: info.IsDir(),
		events:    make(chan machine.FileWatchEvent, 8),
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
	}
	m.watchMu.Lock()
	if m.closed {
		m.watchMu.Unlock()
		_ = watcher.Close()
		return nil, fmt.Errorf("machine-local: watcher is closed")
	}
	m.watches[watch] = struct{}{}
	m.watchMu.Unlock()

	go watch.run()
	return watch, nil
}

func (w *localWatch) Events() <-chan machine.FileWatchEvent { return w.events }

func (w *localWatch) Close() error {
	var closeErr error
	w.closeOnce.Do(func() {
		close(w.stop)
		closeErr = w.watcher.Close()
	})
	<-w.done
	if closeErr != nil && !errors.Is(closeErr, fsnotify.ErrClosed) {
		return closeErr
	}
	return nil
}

func (w *localWatch) run() {
	defer func() {
		_ = w.watcher.Close()
		w.owner.watchMu.Lock()
		delete(w.owner.watches, w)
		w.owner.watchMu.Unlock()
		close(w.events)
		close(w.done)
	}()

	changed := make(map[string]struct{})
	var timer *time.Timer
	var timerC <-chan time.Time
	for {
		select {
		case <-w.stop:
			if timer != nil {
				timer.Stop()
			}
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			path := filepath.Clean(event.Name)
			changedPath, included := w.changedPath(path)
			if err := w.refreshWatch(path); err != nil {
				w.send(machine.FileWatchEvent{Error: err})
				return
			}
			if !included {
				continue
			}
			changed[changedPath] = struct{}{}
			if timer == nil {
				timer = time.NewTimer(watchDebounce)
				timerC = timer.C
			} else if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(watchDebounce)
			timerC = timer.C
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.send(machine.FileWatchEvent{Error: fmt.Errorf("machine-local: watch %q: %w", w.target, err)})
			return
		case <-timerC:
			paths := make([]string, 0, len(changed))
			for path := range changed {
				paths = append(paths, path)
			}
			sort.Strings(paths)
			changed = make(map[string]struct{})
			timerC = nil
			w.send(machine.FileWatchEvent{ChangedPaths: paths})
		}
	}
}

func (w *localWatch) changedPath(path string) (string, bool) {
	if samePath(path, w.target) {
		return w.target, true
	}
	if samePath(path, w.source) {
		return w.target, true
	}
	if w.directory && samePath(filepath.Dir(path), w.source) {
		return filepath.Join(w.target, filepath.Base(path)), true
	}
	return "", false
}

func (w *localWatch) refreshWatch(path string) error {
	if !samePath(path, w.target) && !samePath(path, w.source) {
		return nil
	}
	info, err := os.Stat(w.target)
	if errors.Is(err, os.ErrNotExist) {
		w.source = w.target
		w.directory = false
		return nil
	}
	if err != nil {
		return fmt.Errorf("machine-local: inspect watch target %q: %w", w.target, err)
	}
	source, err := filepath.EvalSymlinks(w.target)
	if err != nil {
		return fmt.Errorf("machine-local: resolve watch target %q: %w", w.target, err)
	}
	source = filepath.Clean(source)
	if err = w.watcher.Add(filepath.Dir(source)); err != nil {
		return fmt.Errorf("machine-local: restore watch parent %q: %w", filepath.Dir(source), err)
	}
	if info.IsDir() {
		if err = w.watcher.Add(source); err != nil {
			return fmt.Errorf("machine-local: restore watch directory %q: %w", source, err)
		}
	}
	w.source = source
	w.directory = info.IsDir()
	return nil
}

func (w *localWatch) send(event machine.FileWatchEvent) {
	select {
	case <-w.stop:
	case w.events <- event:
	}
}

func samePath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}
