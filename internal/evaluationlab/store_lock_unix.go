//go:build linux

package evaluationlab

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func (s *FileStore) acquireLock() error {
	if _, err := s.root.Lstat("writer.lock"); err == nil {
		if err = s.safe("writer.lock"); err != nil {
			return err
		}
	}
	f, err := s.root.OpenFile("writer.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return fmt.Errorf("history already has a writer: %w", err)
	}
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	s.lock = f
	return nil
}

// Rename relative to already opened contained directories. A concurrent local
// rename cannot redirect either parent outside the root between check and use.
func (s *FileStore) rename(oldname, newname string) error {
	oldParent, err := s.root.Open(filepath.Dir(oldname))
	if err != nil {
		return err
	}
	defer oldParent.Close()
	newParent, err := s.root.Open(filepath.Dir(newname))
	if err != nil {
		return err
	}
	defer newParent.Close()
	return syscall.Renameat(int(oldParent.Fd()), filepath.Base(oldname), int(newParent.Fd()), filepath.Base(newname))
}
