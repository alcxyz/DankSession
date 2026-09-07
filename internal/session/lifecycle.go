package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

var ErrBusy = errors.New("another DankSession operation is in progress")

// LockOperation coordinates the daemon and independent CLI/widget processes.
// Never unlink lock files: replacing the inode would allow concurrent owners.
func (m *Manager) LockOperation() (func(), error) {
	return lockFile(m.StatePath + ".lock")
}

func (m *Manager) LockDaemon() (func(), error) {
	return lockFile(m.StatePath + ".daemon.lock")
}

func (m *Manager) DaemonRunning() (bool, error) {
	file, err := os.OpenFile(m.StatePath+".daemon.lock", os.O_RDWR, 0)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer file.Close()
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return true, nil
	}
	return false, err
}

func lockFile(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrBusy
		}
		return nil, err
	}
	return func() { _ = file.Close() }, nil
}

// PrepareSession retains the incoming snapshot before any startup capture.
// A persisted instance marker prevents a daemon restart from restoring again.
func (m *Manager) PrepareSession(instance string) (bool, error) {
	unlock, err := m.LockOperation()
	if err != nil {
		return false, err
	}
	defer unlock()
	marker := m.StatePath + ".instance.json"
	data, err := os.ReadFile(marker)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	var previous string
	if len(data) > 0 {
		if err := json.Unmarshal(data, &previous); err != nil {
			return false, err
		}
	}
	if previous == instance {
		return false, nil
	}
	snapshot, err := m.LoadSnapshot()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err == nil {
		if err := writeJSONAtomic(m.StatePath+".previous", snapshot); err != nil {
			return false, err
		}
	}
	if err := writeJSONAtomic(marker, instance); err != nil {
		return false, err
	}
	return true, nil
}
