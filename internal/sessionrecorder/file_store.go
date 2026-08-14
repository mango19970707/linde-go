package sessionrecorder

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type fileStore struct {
	dir string
}

var safeSessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

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
	path, err := s.pathFor(sessionID)
	if err != nil {
		return sessionRecord{}, err
	}

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

	path, err := s.pathFor(record.SessionID)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

func (s *fileStore) pathFor(sessionID string) (string, error) {
	if err := validateSessionID(sessionID); err != nil {
		return "", err
	}
	return filepath.Join(s.dir, sessionID+".json"), nil
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

	return atomicReplaceFile(tempPath, path)
}

func validateSessionID(sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ErrInvalidSessionID
	}
	if !safeSessionIDPattern.MatchString(sessionID) {
		return fmt.Errorf("%w: %q", ErrInvalidSessionID, sessionID)
	}
	if sessionID == "." || sessionID == ".." {
		return fmt.Errorf("%w: %q", ErrInvalidSessionID, sessionID)
	}
	if isReservedWindowsBaseName(sessionID) {
		return fmt.Errorf("%w: %q", ErrInvalidSessionID, sessionID)
	}
	return nil
}

func isReservedWindowsBaseName(sessionID string) bool {
	base := strings.ToUpper(sessionID)
	if dot := strings.IndexByte(base, '.'); dot >= 0 {
		base = base[:dot]
	}

	switch base {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	default:
		return false
	}
}
