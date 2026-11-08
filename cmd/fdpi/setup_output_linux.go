package main

import (
	"strings"
	"sync"
)

// setupTail keeps only the most recent process output while the terminal shows it live.
type setupTail struct {
	mu   sync.Mutex
	data []byte
}

func (t *setupTail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	size := len(p)
	const limit = 16384
	if size >= limit {
		t.data = append(t.data[:0], p[size-limit:]...)
	} else {
		t.data = append(t.data, p...)
		if len(t.data) > limit {
			t.data = append([]byte(nil), t.data[len(t.data)-limit:]...)
		}
	}
	return size, nil
}
func (t *setupTail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := strings.ToValidUTF8(string(t.data), "")
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > 8 {
		lines = lines[len(lines)-8:]
	}
	return strings.Join(lines, "\n")
}
