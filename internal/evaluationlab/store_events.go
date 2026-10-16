package evaluationlab

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const maxStoredEvents = 16384

// StoreEvent is a content-free durable checkpoint observation, independent of
// the runner's bounded live stage-event ring. It never invents provider timing.
type StoreEvent struct {
	Sequence    uint64      `json:"sequence"`
	EngineState string      `json:"engineState"`
	Progress    RunProgress `json:"progress"`
	Dispatches  int         `json:"dispatches"`
}

func (s *FileStore) readEvents(id string) ([]StoreEvent, error) {
	raw, err := s.read(filepath.Join(id, "events.jsonl"), 8<<20)
	if err != nil {
		return nil, err
	}
	if len(raw) > 0 && raw[len(raw)-1] != '\n' {
		return nil, fmt.Errorf("truncated event journal")
	}
	events := []StoreEvent{}
	for _, line := range bytes.Split(raw, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var event StoreEvent
		if len(events) >= maxStoredEvents || len(line) > 4096 {
			return nil, fmt.Errorf("event journal exceeds bounds")
		}
		if err = json.Unmarshal(line, &event); err != nil {
			return nil, err
		}
		if event.Sequence != uint64(len(events)+1) {
			return nil, fmt.Errorf("event sequence gap")
		}
		events = append(events, event)
	}
	return events, nil
}
func (s *FileStore) appendCheckpoint(record RunRecord) error {
	events, err := s.readEvents(record.ID)
	if os.IsNotExist(err) {
		events = nil
	} else if err != nil {
		return err
	}
	if len(events) >= maxStoredEvents {
		return fmt.Errorf("event journal capacity reached")
	}
	event := StoreEvent{Sequence: uint64(len(events) + 1), EngineState: record.EngineState, Progress: record.Progress, Dispatches: len(record.Dispatches)}
	raw, _ := json.Marshal(event)
	raw = append(raw, '\n')
	if err = s.guard("append-event"); err != nil {
		return err
	}
	path := filepath.Join(record.ID, "events.jsonl")
	if _, err = s.root.Lstat(path); err == nil {
		if err = s.safe(path); err != nil {
			return err
		}
	}
	f, err := s.root.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(raw)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return s.syncDir(record.ID)
}
func (s *FileStore) Events(id string, after uint64, limit int) ([]StoreEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, fmt.Errorf("store closed")
	}
	if _, err := s.manifest(id); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 256 {
		return nil, fmt.Errorf("invalid event limit")
	}
	events, err := s.readEvents(id)
	if err != nil {
		return nil, err
	}
	if after > uint64(len(events)) {
		return nil, fmt.Errorf("event cursor beyond journal")
	}
	end := min(int(after)+limit, len(events))
	return events[int(after):end], nil
}
