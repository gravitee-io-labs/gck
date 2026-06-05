package dns

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"k8s.io/klog/v2"
)

// RecordFile is the on-disk format for a cluster's DNS records.
// Each cluster gets its own file (<cluster-name>.json) in the records
// directory (.gck/dns/).
type RecordFile struct {
	Records map[string]string `json:"records"`
}

// RecordStore maintains an aggregated hostname-to-IP map built from
// per-cluster JSON record files. It watches the directory for changes and
// signals via a channel when no record files remain (triggering auto-shutdown).
type RecordStore struct {
	dir    string
	mu     sync.RWMutex
	byFile map[string]RecordFile // filename → parsed records
	merged map[string]string     // hostname (lowercase) → IP
	empty  chan struct{}          // closed when directory becomes empty
}

// NewRecordStore creates a store backed by the given directory.
func NewRecordStore(dir string) *RecordStore {
	return &RecordStore{
		dir:    dir,
		byFile: make(map[string]RecordFile),
		merged: make(map[string]string),
		empty:  make(chan struct{}),
	}
}

// Load reads all .json files from the directory and rebuilds the aggregated
// hostname-to-IP map. It returns an error if the directory cannot be read but
// tolerates individual malformed files (logging a warning for each).
func (s *RecordStore) Load() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return fmt.Errorf("reading record directory %s: %w", s.dir, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.byFile = make(map[string]RecordFile)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		rf, err := loadRecordFile(filepath.Join(s.dir, e.Name()))
		if err != nil {
			klog.Warningf("skipping record file %s: %v", e.Name(), err)
			continue
		}
		s.byFile[e.Name()] = rf
	}
	s.rebuild()
	s.checkEmpty()
	return nil
}

// Lookup returns the IP for hostname if any record matches.
// The lookup is case-insensitive. When no exact match is found, it falls back
// to wildcard matching per RFC 4592: the first DNS label is replaced with "*"
// and tried again (e.g. "foo.kafka.gck.local" matches "*.kafka.gck.local").
func (s *RecordStore) Lookup(hostname string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	lower := strings.ToLower(hostname)
	if ip, ok := s.merged[lower]; ok {
		return ip, true
	}
	if idx := strings.IndexByte(lower, '.'); idx >= 0 {
		if ip, ok := s.merged["*"+lower[idx:]]; ok {
			return ip, true
		}
	}
	return "", false
}

// Empty returns a channel that is closed when the record directory contains
// no more .json files. Callers can select on this to trigger auto-shutdown.
func (s *RecordStore) Empty() <-chan struct{} {
	return s.empty
}

// RecordCount returns the number of hostnames currently resolved.
func (s *RecordStore) RecordCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.merged)
}

// Watch starts watching the record directory for changes. It reloads
// affected files on create/write/remove events and updates the merged
// map. It blocks until ctx is cancelled or the watcher encounters a
// fatal error.
func (s *RecordStore) Watch(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("creating file watcher: %w", err)
	}
	defer watcher.Close()

	if err := watcher.Add(s.dir); err != nil {
		return fmt.Errorf("watching %s: %w", s.dir, err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if !strings.HasSuffix(event.Name, ".json") {
				continue
			}
			s.handleEvent(event)
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			klog.Warningf("file watcher error: %v", err)
		}
	}
}

func (s *RecordStore) handleEvent(event fsnotify.Event) {
	name := filepath.Base(event.Name)

	s.mu.Lock()
	defer s.mu.Unlock()

	switch {
	case event.Op&(fsnotify.Create|fsnotify.Write) != 0:
		rf, err := loadRecordFile(event.Name)
		if err != nil {
			klog.Warningf("reloading record file %s: %v", name, err)
			return
		}
		klog.V(2).Infof("loaded record file %s (%d records)", name, len(rf.Records))
		s.byFile[name] = rf
	case event.Op&(fsnotify.Remove|fsnotify.Rename) != 0:
		klog.V(2).Infof("removed record file %s", name)
		delete(s.byFile, name)
	}

	s.rebuild()
	s.checkEmpty()
}

// checkEmpty closes the empty channel if no record files remain.
// Caller must hold s.mu.
func (s *RecordStore) checkEmpty() {
	if len(s.byFile) == 0 {
		select {
		case <-s.empty:
			// already closed
		default:
			close(s.empty)
		}
	}
}

// rebuild recomputes the merged hostname-to-IP map from all loaded files.
// Caller must hold s.mu.
func (s *RecordStore) rebuild() {
	s.merged = make(map[string]string, len(s.byFile)*4)
	for _, rf := range s.byFile {
		for hostname, ip := range rf.Records {
			s.merged[strings.ToLower(hostname)] = ip
		}
	}
}

func loadRecordFile(path string) (RecordFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RecordFile{}, err
	}
	var rf RecordFile
	if err := json.Unmarshal(data, &rf); err != nil {
		return RecordFile{}, fmt.Errorf("parsing %s: %w", filepath.Base(path), err)
	}
	return rf, nil
}

// WriteRecordFile writes a record file for the given cluster to the records
// directory. It creates the directory if it does not exist.
func WriteRecordFile(dir, cluster string, records map[string]string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating record directory: %w", err)
	}
	rf := RecordFile{Records: records}
	data, err := json.MarshalIndent(rf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling record file: %w", err)
	}
	data = append(data, '\n')
	path := filepath.Join(dir, cluster+".json")
	return os.WriteFile(path, data, 0o644)
}

// RemoveRecordFile removes the record file for the given cluster.
// It returns nil if the file does not exist.
func RemoveRecordFile(dir, cluster string) error {
	path := filepath.Join(dir, cluster+".json")
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing record file %s: %w", path, err)
	}
	return nil
}
