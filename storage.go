package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FileStorage writes JSON Lines so multiple workers can append independently.
type FileStorage struct {
	mu   sync.Mutex
	file *os.File
}

func NewFileStorage(dir, workerID string) (*FileStorage, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, fmt.Sprintf("results-%s.jsonl", workerID))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &FileStorage{file: f}, nil
}

func (s *FileStorage) Save(_ context.Context, result CrawlResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	b, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if _, err := s.file.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

func (s *FileStorage) Close() error {
	if s.file == nil {
		return nil
	}
	return s.file.Close()
}
