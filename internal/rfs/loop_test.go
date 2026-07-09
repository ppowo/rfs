package rfs

import (
	"context"
	"testing"
	"time"
)

// TestLoopDrainsInFlightPollBeforeReturning verifies that on shutdown an
// in-flight poll is allowed to finish — the fetch completes, the snapshot
// save commits — rather than being hard-cut. This is the drain contract the
// self-update exec path and a SIGTERM both depend on.
func TestLoopDrainsInFlightPollBeforeReturning(t *testing.T) {
	started := make(chan struct{})
	proceed := make(chan struct{})
	pollDone := make(chan struct{})
	poll := func(context.Context) {
		close(started)
		<-proceed // block until the test releases the in-flight poll
		close(pollDone)
	}
	loop := Loop{Poll: poll, Interval: time.Hour}

	ctx, cancel := context.WithCancel(context.Background())
	runReturned := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(runReturned)
	}()

	// Wait for the initial poll to be in-flight.
	<-started

	// Shut down. Run must NOT return while the poll is still running.
	cancel()
	select {
	case <-runReturned:
		t.Fatal("Run returned before the in-flight poll finished (drain broken)")
	case <-time.After(50 * time.Millisecond):
		// Good: still draining.
	}

	// Release the poll. Now Run should return.
	close(proceed)
	select {
	case <-runReturned:
		// Good.
	case <-time.After(time.Second):
		t.Fatal("Run did not return after the in-flight poll finished")
	}
	<-pollDone
}

// TestLoopTerminatesDrainWhenPollIsStuck verifies that a stuck in-flight
// poll (one that never completes on its own) cannot pin shutdown forever:
// the per-poll context's timeout cuts it, so Run returns within the bound.
// Without this, a hung upstream would leave the process unable to exit or
// re-exec — drain would wait on a poll that never finishes.
func TestLoopTerminatesDrainWhenPollIsStuck(t *testing.T) {
	pollStarted := make(chan struct{})
	poll := func(ctx context.Context) {
		close(pollStarted)
		<-ctx.Done() // a stuck poll that only stops when its context is cancelled
	}
	loop := Loop{Poll: poll, Interval: time.Hour, PollTimeout: 50 * time.Millisecond}

	ctx, cancel := context.WithCancel(context.Background())
	runReturned := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(runReturned)
	}()

	<-pollStarted
	cancel() // request shutdown while the poll is stuck

	select {
	case <-runReturned:
		// Good: the stuck poll was cut by its own per-poll timeout, so the
		// drain terminated and Run returned.
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after shutdown; a stuck poll pinned the drain (drain never terminates)")
	}
}
