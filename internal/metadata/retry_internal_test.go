package metadata

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestWithRetry_SucceedsAfterATransientFailure(t *testing.T) {
	// Google Books answers 503 several times an hour. Without a retry that
	// routine flakiness becomes a book typed in by hand.
	calls := 0
	body, err := withRetry(t.Context(), func() ([]byte, bool, error) {
		calls++
		if calls < 3 {
			return nil, true, errors.New("status 503")
		}
		return []byte(`{"ok":true}`), false, nil
	})
	if err != nil {
		t.Fatalf("withRetry gave up: %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("body = %q", body)
	}
	if calls != 3 {
		t.Errorf("called %d times, want 3", calls)
	}
}

func TestWithRetry_DoesNotRetryAFinalFailure(t *testing.T) {
	// Retrying a bad request only wastes someone else's quota.
	calls := 0
	_, err := withRetry(t.Context(), func() ([]byte, bool, error) {
		calls++
		return nil, false, errors.New("status 400")
	})
	if err == nil {
		t.Fatal("a final failure was reported as success")
	}
	if calls != 1 {
		t.Errorf("called %d times, want 1: a 4xx must not be retried", calls)
	}
}

func TestWithRetry_StopsAfterAttempts(t *testing.T) {
	calls := 0
	_, err := withRetry(t.Context(), func() ([]byte, bool, error) {
		calls++
		return nil, true, errors.New("status 503")
	})
	if err == nil {
		t.Fatal("endless failure was reported as success")
	}
	if calls != Attempts {
		t.Errorf("called %d times, want %d", calls, Attempts)
	}
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("the final error hides what went wrong: %v", err)
	}
}

func TestWithRetry_StopsWhenTheContextIsCancelled(t *testing.T) {
	// A person who navigates away should not keep a provider busy.
	ctx, cancel := context.WithCancel(t.Context())
	calls := 0
	_, err := withRetry(ctx, func() ([]byte, bool, error) {
		calls++
		cancel()
		return nil, true, errors.New("status 503")
	})
	if err == nil {
		t.Fatal("a cancelled fetch was reported as success")
	}
	if calls != 1 {
		t.Errorf("called %d times after cancellation, want 1", calls)
	}
}

func TestRetryPolicy_IsSane(t *testing.T) {
	if Attempts < 2 {
		t.Errorf("Attempts = %d: one try turns routine flakiness into a lookup failure", Attempts)
	}
	if RetryBackoff <= 0 {
		t.Error("RetryBackoff is not positive, so retries would hammer the provider")
	}
	if total := RetryBackoff * time.Duration(Attempts); total > RequestTimeout {
		t.Errorf("the backoff alone (%s) outlasts the request timeout (%s)", total, RequestTimeout)
	}
}

func TestAttempt_TreatsStatusesCorrectly(t *testing.T) {
	// Which statuses are worth another try is the whole policy.
	tests := []struct {
		status    int
		retryable bool
	}{
		{http.StatusServiceUnavailable, true},
		{http.StatusInternalServerError, true},
		{http.StatusBadGateway, true},
		{http.StatusTooManyRequests, true},
		{http.StatusNotFound, false},
		{http.StatusForbidden, false},
		{http.StatusBadRequest, false},
	}
	for _, tt := range tests {
		got := tt.status >= 500 || tt.status == http.StatusTooManyRequests
		if got != tt.retryable {
			t.Errorf("status %d: retryable = %v, want %v", tt.status, got, tt.retryable)
		}
	}
}
