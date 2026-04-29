package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Store persists sessions as JSON files under Dir.
type Store struct {
	Dir string
}

// NewStore creates a Store pointed at ~/.kubewise/sessions/, creating it if absent.
func NewStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}
	dir := filepath.Join(home, ".kubewise", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create sessions dir: %w", err)
	}
	return &Store{Dir: dir}, nil
}

// Save writes sess to a JSON file named <date>-<id>.json, creating the dir if needed.
func (s *Store) Save(sess *Session) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return fmt.Errorf("create sessions dir: %w", err)
	}
	sess.UpdatedAt = time.Now()
	filename := fmt.Sprintf("%s-%s.json", sess.CreatedAt.Format("2006-01-02-150405"), sess.ID)
	path := filepath.Join(s.Dir, filename)
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadRecent returns up to n sessions sorted by file modification time, newest first.
func (s *Store) LoadRecent(n int) ([]*Session, error) {
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

	sessions := make([]*Session, 0, len(files))
	for _, f := range files {
		data, err := os.ReadFile(f.path)
		if err != nil {
			continue
		}
		var sess Session
		if err := json.Unmarshal(data, &sess); err != nil {
			continue
		}
		sessions = append(sessions, &sess)
	}
	return sessions, nil
}
