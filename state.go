package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type FileState struct {
	Source    string `json:"source"`
	Path      string `json:"path"`
	Offset    int64  `json:"offset"`
	FileSize  int64  `json:"file_size"`
	UpdatedAt string `json:"updated_at"`
}

type StateStore struct {
	mu    sync.Mutex
	path  string
	Files map[string]FileState `json:"files"`
}

func LoadState(path string) (*StateStore, error) {
	store := &StateStore{
		path:  path,
		Files: map[string]FileState{},
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return nil, fmt.Errorf("read state %s: %w", path, err)
	}
	if len(data) == 0 {
		return store, nil
	}
	if err := json.Unmarshal(data, store); err != nil {
		return nil, fmt.Errorf("parse state %s: %w", path, err)
	}
	if store.Files == nil {
		store.Files = map[string]FileState{}
	}
	store.path = path
	return store, nil
}

func (s *StateStore) Get(sourceName, path string) (FileState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.Files[stateKey(sourceName, path)]
	return state, ok
}

func (s *StateStore) Set(sourceName, path string, offset, fileSize int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Files[stateKey(sourceName, path)] = FileState{
		Source:    sourceName,
		Path:      filepath.Clean(path),
		Offset:    offset,
		FileSize:  fileSize,
		UpdatedAt: time.Now().Format(time.RFC3339),
	}
}

func (s *StateStore) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write temp state: %w", err)
	}
	_ = os.Remove(s.path)
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("replace state: %w", err)
	}
	return nil
}

func stateKey(sourceName, path string) string {
	return sourceName + "|" + filepath.Clean(path)
}
