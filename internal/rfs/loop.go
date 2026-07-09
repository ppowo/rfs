package rfs

import (
	"context"
	"sync"
	"time"
)

// Loop polls on a fixed interval until ctx is cancelled. On cancellation it
// waits for an in-flight poll to finish (drain) rather than hard-cutting it,
// so a snapshot save in progress is allowed to commit before the process
// exits or re-execs. Each poll runs under a per-poll timeout (PollTimeout,
// default 2m) on a context independent of the shutdown signal — a
// cancellation stops the next poll from starting, never the one in flight,
// and a stuck poll is cut by its own timeout so drain always terminates.
type Loop struct {
	Poll     func(context.Context)
	Interval time.Duration

	// PollTimeout bounds a single poll. A stuck in-flight poll is cut by its
	// own per-poll context — never by the shutdown signal — so shutdown drain
	// always terminates even if an upstream hangs. Defaults to 2 minutes
	// when zero, generous for the static single-page sources rfs polls.
	PollTimeout time.Duration
}

func (l Loop) Run(ctx context.Context) {
	pollTimeout := l.PollTimeout
	if pollTimeout <= 0 {
		pollTimeout = 2 * time.Minute
	}
	var wg sync.WaitGroup
	poll := func() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// The poll context is unrelated to the shutdown signal: a
			// cancellation drains (the in-flight poll runs to completion)
			// instead of aborting it. Its own timeout cuts a stuck poll.
			pollCtx, cancel := context.WithTimeout(context.Background(), pollTimeout)
			defer cancel()
			l.Poll(pollCtx)
		}()
	}

	poll()
	ticker := time.NewTicker(l.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			wg.Wait() // drain the in-flight poll before returning
			return
		case <-ticker.C:
			poll()
		}
	}
}
