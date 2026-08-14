package sessionrecorder

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type fileStore struct {
	dir string
}

type sessionRecord struct {
	SessionID  string     `json:"session_id"`
	Status     string     `json:"status"`
	Transcript Transcript `json:"transcript"`
}

func newFileStore(dir string) (*fileStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	return &fileStore{dir: dir}, nil
}

func (s *fileStore) load(sessionID string) (sessionRecord, error) {
	path := s.pathFor(sessionID)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return sessionRecord{
				SessionID:  sessionID,
				Transcript: Transcript{},
			}, nil
		}
		return sessionRecord{}, err
	}

	var record sessionRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return sessionRecord{}, err
	}
	if record.SessionID == "" {
		record.SessionID = sessionID
	}
	if record.Transcript == nil {
		record.Transcript = Transcript{}
	}
	return record, nil
}

func (s *fileStore) save(record sessionRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}

	path := s.pathFor(record.SessionID)
	return writeFileAtomic(path, data)
}

func (s *fileStore) pathFor(sessionID string) string {
	return filepath.Join(s.dir, sessionID+".json")
}

func writeFileAtomic(path string, data []byte) (err error) {
	dir := filepath.Dir(path)

	file, err := os.CreateTemp(dir, ".session-*.tmp")
	if err != nil {
		return err
	}

	tempPath := file.Name()
	defer func() {
		if err == nil {
			return
		}
		_ = os.Remove(tempPath)
	}()

	if _, err = file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}

	if err = os.Rename(tempPath, path); err == nil {
		return nil
	}

	if !errors.Is(err, os.ErrExist) {
		return err
	}
	if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		return err
	}
	return os.Rename(tempPath, path)
}
