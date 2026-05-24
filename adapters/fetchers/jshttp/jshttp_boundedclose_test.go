package jshttp

// Unit tests for the closeWithTimeout helper.
//
// These tests verify the core bounded-close contract without launching a
// real browser: a hung Close is abandoned after the deadline and the calling
// goroutine is freed.  Run as part of the normal (non-integration) test suite.

import (
	"errors"
	"testing"
	"time"
)

// TestCloseWithTimeout_Normal verifies that a fast Close finishes before
// the deadline and no extra delay is introduced.
func TestCloseWithTimeout_Normal(t *testing.T) {
	called := false
	closer := func() error {
		called = true
		return nil
	}

	start := time.Now()
	closeWithTimeout(closer, 5*time.Second)
	elapsed := time.Since(start)

	if !called {
		t.Error("closer was not called")
	}
	// Should complete well under the deadline (normal close is near-instant here).
	if elapsed > 2*time.Second {
		t.Errorf("closeWithTimeout took %v for a fast closer; expected < 2s", elapsed)
	}
}

// TestCloseWithTimeout_HungClose verifies that a closer that never returns
// is abandoned after the deadline so the calling goroutine is not blocked
// indefinitely.  This is the core bounded-close contract (plan §B2, Befund 3).
func TestCloseWithTimeout_HungClose(t *testing.T) {
	deadline := 100 * time.Millisecond // use a short deadline for unit-test speed

	hung := func() error {
		// Block forever — simulates a wedged Playwright driver (e.g. EPIPE).
		select {}
	}

	start := time.Now()
	closeWithTimeout(hung, deadline)
	elapsed := time.Since(start)

	// The call must return close to the deadline — not block indefinitely.
	// Allow 3× the deadline as generous tolerance for scheduling jitter.
	max := 3 * deadline
	if elapsed > max {
		t.Errorf("closeWithTimeout blocked for %v (deadline=%v, max=%v): hung Close was not abandoned",
			elapsed, deadline, max)
	}
}

// TestCloseWithTimeout_ErrorIgnored verifies that a Close returning an error
// still completes without panicking or propagating the error.
func TestCloseWithTimeout_ErrorIgnored(t *testing.T) {
	errCloser := func() error {
		return errors.New("driver error")
	}
	// Must not panic.
	closeWithTimeout(errCloser, time.Second)
}
