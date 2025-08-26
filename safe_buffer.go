package goserver

import (
	"bytes"
	"io"
	"sync"
)

// safeBuffer is a concurrency-safe buffer intended for tests and logging.
// It implements io.Writer and provides String/Reset methods protected by a mutex.
type safeBuffer struct {
	mu sync.Mutex
	// If w is non-nil, writes are forwarded to w (wrapped writer).
	// Otherwise, writes go to the internal buffer `buf` which supports
	// String/Reset for test inspection.
	w   io.Writer
	buf bytes.Buffer
}

// Write implements io.Writer, safe for concurrent use.
func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.w != nil {
		return s.w.Write(p)
	}
	return s.buf.Write(p)
}

// String returns the buffer contents safely.
func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// Reset clears the buffer safely.
func (s *safeBuffer) Reset() {
	s.mu.Lock()
	if s.w == nil {
		s.buf.Reset()
	}
	s.mu.Unlock()
}

// ensureSyncWriter wraps the provided io.Writer with a *safeBuffer that
// serializes writes. If the provided writer is nil, it returns nil. If the
// writer is already a *safeBuffer, it is returned as-is.
func ensureSyncWriter(w io.Writer) io.Writer {
	if w == nil {
		return nil
	}
	if sb, ok := w.(*safeBuffer); ok {
		return sb
	}
	return &safeBuffer{w: w}
}
