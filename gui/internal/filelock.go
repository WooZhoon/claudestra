package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type LockConflictError struct {
	Path      string
	Holder    string
	Requester string
}

func (e *LockConflictError) Error() string {
	return fmt.Sprintf("'%s' 는 '%s' 에이전트가 사용 중 (요청: '%s')", e.Path, e.Holder, e.Requester)
}

type FileLockRegistry struct {
	locksDir string
	locks    map[string]string // normalized_path → agent_id
	mu       sync.Mutex
}

func NewFileLockRegistry(locksDir string) *FileLockRegistry {
	os.MkdirAll(locksDir, 0755)
	r := &FileLockRegistry{
		locksDir: locksDir,
		locks:    make(map[string]string),
	}
	r.load()
	return r
}

func (r *FileLockRegistry) Acquire(filePath, agentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	normalized, _ := filepath.Abs(filePath)
	if holder, ok := r.locks[normalized]; ok && holder != agentID {
		return &LockConflictError{Path: normalized, Holder: holder, Requester: agentID}
	}
	r.locks[normalized] = agentID
	r.save()
	return nil
}

func (r *FileLockRegistry) Release(filePath, agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	normalized, _ := filepath.Abs(filePath)
	if r.locks[normalized] == agentID {
		delete(r.locks, normalized)
		r.save()
	}
}

func (r *FileLockRegistry) ReleaseAll(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for path, holder := range r.locks {
		if holder == agentID {
			delete(r.locks, path)
		}
	}
	r.save()
}

func (r *FileLockRegistry) HeldBy(filePath string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	normalized, _ := filepath.Abs(filePath)
	holder, ok := r.locks[normalized]
	return holder, ok
}

func (r *FileLockRegistry) ListLocks() map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()

	result := make(map[string]string, len(r.locks))
	for k, v := range r.locks {
		result[k] = v
	}
	return result
}

func (r *FileLockRegistry) save() {
	data, _ := json.MarshalIndent(r.locks, "", "  ")
	os.WriteFile(filepath.Join(r.locksDir, "registry.json"), data, 0644)
}

func (r *FileLockRegistry) load() {
	data, err := os.ReadFile(filepath.Join(r.locksDir, "registry.json"))
	if err != nil {
		return
	}
	json.Unmarshal(data, &r.locks)
}
