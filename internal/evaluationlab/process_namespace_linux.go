//go:build linux

package evaluationlab

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"
)

// Bubblewrap's structured status identifies the owned PID-namespace init, not
// a user-supplied PID. Killing that init terminates every detached descendant.
type processNamespace struct {
	mu    sync.Mutex
	pid   int
	inode uint64
	done  chan struct{}
}

func watchProcessNamespace(reader *os.File) *processNamespace {
	n := &processNamespace{done: make(chan struct{})}
	go func() {
		defer close(n.done)
		decoder := json.NewDecoder(io.LimitReader(reader, 8192))
		for {
			var status struct {
				PID   int    `json:"child-pid"`
				Inode uint64 `json:"pid-namespace"`
			}
			if decoder.Decode(&status) != nil {
				return
			}
			if status.PID > 0 && status.Inode > 0 {
				n.mu.Lock()
				n.pid = status.PID
				n.inode = status.Inode
				n.mu.Unlock()
			}
		}
	}()
	return n
}
func (n *processNamespace) stop(reader *os.File) bool {
	select {
	case <-n.done:
	case <-time.After(500 * time.Millisecond):
		reader.Close()
	}
	n.mu.Lock()
	pid, inode := n.pid, n.inode
	n.mu.Unlock()
	if pid < 1 || inode == 0 {
		return false
	}
	path := "/proc/" + strconv.Itoa(pid) + "/ns/pid"
	expected := fmt.Sprintf("pid:[%d]", inode)
	owned, findErr := os.FindProcess(pid)
	if findErr != nil {
		return false
	}
	defer owned.Release()
	for i := 0; i < 30; i++ {
		current, err := os.Readlink(path)
		if os.IsNotExist(err) || err == nil && current != expected {
			return true
		}
		if err != nil {
			return false
		}
		// A retained pidfd prevents PID reuse between namespace verification and kill.
		if !hasProcessHandle(owned) {
			return false
		}
		_ = owned.Kill()
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// Keep the repository's Go language baseline; older supervisors fail preflight
// rather than fall back to a racy numeric-PID signal.
func hasProcessHandle(process *os.Process) bool {
	handle, ok := any(process).(interface{ WithHandle(func(uintptr)) error })
	return ok && handle.WithHandle(func(uintptr) { /* Acquiring a valid handle is the capability being tested. */ }) == nil
}
