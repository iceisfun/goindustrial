package client

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/iceisfun/goeip/pkg/session"
)

// mockTransport implements Transport for testing.
type mockTransport struct {
	mu          sync.Mutex
	sess        *session.Session
	sessionFunc func() (*session.Session, error)
	resetFunc   func(stale *session.Session) error
	closeFunc   func() error
}

func (m *mockTransport) Session() (*session.Session, error) {
	if m.sessionFunc != nil {
		return m.sessionFunc()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sess, nil
}

func (m *mockTransport) Reset(stale *session.Session) error {
	if m.resetFunc != nil {
		return m.resetFunc(stale)
	}
	return nil
}

func (m *mockTransport) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func TestRetry_SucceedsAfterFailures(t *testing.T) {
	attempts := 0
	mt := &mockTransport{
		sessionFunc: func() (*session.Session, error) {
			return &session.Session{}, nil
		},
	}

	c := NewClient(mt,
		WithRetries(5),
		WithRetryDelay(1*time.Millisecond),
		WithLogger(&MockLogger{}),
	)

	err := c.do(func(sess *session.Session) error {
		attempts++
		if attempts < 3 {
			return fmt.Errorf("transport error")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetry_NoRetries(t *testing.T) {
	attempts := 0
	mt := &mockTransport{
		sessionFunc: func() (*session.Session, error) {
			return &session.Session{}, nil
		},
	}

	c := NewClient(mt, WithRetries(0), WithLogger(&MockLogger{}))

	err := c.do(func(sess *session.Session) error {
		attempts++
		return fmt.Errorf("fail")
	})

	if err == nil {
		t.Fatalf("expected error")
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}

func TestRetry_InfiniteRetries(t *testing.T) {
	attempts := 0
	mt := &mockTransport{
		sessionFunc: func() (*session.Session, error) {
			return &session.Session{}, nil
		},
	}

	c := NewClient(mt,
		WithRetries(-1),
		WithRetryDelay(1*time.Microsecond),
		WithLogger(&MockLogger{}),
	)

	err := c.do(func(sess *session.Session) error {
		attempts++
		if attempts < 10 {
			return fmt.Errorf("fail")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if attempts < 10 {
		t.Errorf("expected at least 10 attempts, got %d", attempts)
	}
}

func TestRetry_CIPErrorNotRetried(t *testing.T) {
	attempts := 0
	mt := &mockTransport{
		sessionFunc: func() (*session.Session, error) {
			return &session.Session{}, nil
		},
	}

	c := NewClient(mt,
		WithRetries(5),
		WithRetryDelay(1*time.Millisecond),
		WithLogger(&MockLogger{}),
	)

	err := c.do(func(sess *session.Session) error {
		attempts++
		return &cipError{errors.New("path not found")}
	})

	if err == nil {
		t.Fatalf("expected error")
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retry on CIP error), got %d", attempts)
	}
	if err.Error() != "path not found" {
		t.Errorf("expected unwrapped CIP error, got: %v", err)
	}
}

func TestRetry_ResetCalledWithStaleSession(t *testing.T) {
	sess := &session.Session{}
	var resetSess *session.Session

	mt := &mockTransport{
		sessionFunc: func() (*session.Session, error) {
			return sess, nil
		},
		resetFunc: func(stale *session.Session) error {
			resetSess = stale
			return nil
		},
	}

	c := NewClient(mt,
		WithRetries(1),
		WithRetryDelay(1*time.Millisecond),
		WithLogger(&MockLogger{}),
	)

	c.do(func(s *session.Session) error {
		return fmt.Errorf("transport error")
	})

	if resetSess != sess {
		t.Errorf("Reset was not called with the stale session")
	}
}

func TestRetry_SessionUnavailableRetries(t *testing.T) {
	attempts := 0
	mt := &mockTransport{
		sessionFunc: func() (*session.Session, error) {
			attempts++
			if attempts < 3 {
				return nil, fmt.Errorf("connection refused")
			}
			return &session.Session{}, nil
		},
	}

	c := NewClient(mt,
		WithRetries(5),
		WithRetryDelay(1*time.Millisecond),
		WithLogger(&MockLogger{}),
	)

	err := c.do(func(sess *session.Session) error {
		return nil
	})

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if attempts < 3 {
		t.Errorf("expected at least 3 session attempts, got %d", attempts)
	}
}
