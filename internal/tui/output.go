package tui

import (
	"strings"
	"sync"
)

type operationOutput struct {
	mu   sync.Mutex
	data []byte
}

func (o *operationOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	size := len(p)
	const limit = 65536
	if size >= limit {
		o.data = append(o.data[:0], p[size-limit:]...)
	} else {
		o.data = append(o.data, p...)
		if len(o.data) > limit {
			o.data = append([]byte(nil), o.data[len(o.data)-limit:]...)
		}
	}
	return size, nil
}
func (o *operationOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return strings.ToValidUTF8(string(o.data), "")
}
