//go:build !linux

package evaluationlab

import "fmt"

func (s *FileStore) acquireLock() error {
	return fmt.Errorf("private history writer locking unavailable on this platform")
}

func (s *FileStore) rename(oldname, newname string) error {
	return fmt.Errorf("contained atomic rename unavailable on this platform")
}
