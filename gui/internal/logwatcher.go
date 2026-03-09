package internal

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// LogWatcher watches .orchestra/logs/ for JSONL file changes
// and emits new LogEntry items via a callback.
type LogWatcher struct {
	dir      string
	callback func(LogEntry)
	offsets  map[string]int64 // file → last read position
	mu       sync.Mutex
	stop     chan struct{}
	done     chan struct{}
}

func NewLogWatcher(logsDir string, callback func(LogEntry)) *LogWatcher {
	return &LogWatcher{
		dir:      logsDir,
		callback: callback,
		offsets:  make(map[string]int64),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// Start begins watching. Call Stop() to end.
func (w *LogWatcher) Start() error {
	os.MkdirAll(w.dir, 0755)

	// 기존 파일의 현재 위치를 기록 (이전 내용 무시)
	w.snapshotOffsets()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := watcher.Add(w.dir); err != nil {
		watcher.Close()
		return err
	}

	go func() {
		defer close(w.done)
		defer watcher.Close()

		for {
			select {
			case <-w.stop:
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
					if strings.HasSuffix(event.Name, ".jsonl") {
						w.readNewLines(event.Name)
					}
				}
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()

	return nil
}

func (w *LogWatcher) Stop() {
	close(w.stop)
	<-w.done
}

// snapshotOffsets records current file sizes so we only read new content.
func (w *LogWatcher) snapshotOffsets() {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".jsonl") {
			path := filepath.Join(w.dir, e.Name())
			if info, err := os.Stat(path); err == nil {
				w.offsets[path] = info.Size()
			}
		}
	}
}

// readNewLines reads lines added since last offset.
func (w *LogWatcher) readNewLines(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	offset := w.offsets[path]

	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	if offset > 0 {
		f.Seek(offset, 0)
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var entry LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		w.callback(entry)
	}

	// 현재 위치 기록
	if pos, err := f.Seek(0, 1); err == nil {
		w.offsets[path] = pos
	}
}
