package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kubewise/kubewise/internal/agent/session"
)

// FileStore persists conversations as JSON files on disk.
type FileStore struct {
	Dir string
}

// NewFileStore creates a FileStore pointed at baseDir/sessions/.
func NewFileStore(baseDir string) (*FileStore, error) {
	dir := filepath.Join(baseDir, "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create sessions dir: %w", err)
	}
	return &FileStore{Dir: dir}, nil
}

// Save writes a conversation as a JSON file.
func (s *FileStore) Save(conv *session.Conversation) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return fmt.Errorf("create sessions dir: %w", err)
	}
	conv.UpdatedAt = time.Now()
	filename := fmt.Sprintf("%s-%s.json", conv.CreatedAt.Format("2006-01-02-150405"), conv.ID)
	path := filepath.Join(s.Dir, filename)
	data, err := json.MarshalIndent(conv, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadRecent returns up to n conversations sorted by modification time, newest first.
func (s *FileStore) LoadRecent(n int) ([]*session.Conversation, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	type entry struct {
		path    string
		modTime time.Time
	}
	var files []entry
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, entry{
			path:    filepath.Join(s.Dir, e.Name()),
			modTime: info.ModTime(),
		})
	}

	sort.SliceStable(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})

	if len(files) > n {
		files = files[:n]
	}

	convs := make([]*session.Conversation, 0, len(files))
	for _, f := range files {
		data, err := os.ReadFile(f.path)
		if err != nil {
			continue
		}
		var conv session.Conversation
		if err := json.Unmarshal(data, &conv); err != nil {
			continue
		}
		convs = append(convs, &conv)
	}
	return convs, nil
}

// Delete removes the conversation file matching the given ID.
func (s *FileStore) Delete(id string) error {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		if strings.Contains(e.Name(), id+".json") {
			return os.Remove(filepath.Join(s.Dir, e.Name()))
		}
	}
	return fmt.Errorf("session %s not found", id)
}
