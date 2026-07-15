package main

import (
	"testing"
	"time"
)

func TestProcessTransitionWaitsForPollSchedulesAndServer(t *testing.T) {
	tests := []struct {
		name        string
		finishFirst func(pollsDone, serverDone chan struct{})
		finishLast  func(pollsDone, serverDone chan struct{})
	}{
		{
			name:        "poll schedules drain first",
			finishFirst: func(pollsDone, _ chan struct{}) { close(pollsDone) },
			finishLast:  func(_ chan struct{}, serverDone chan struct{}) { close(serverDone) },
		},
		{
			name:        "server stops first",
			finishFirst: func(_, serverDone chan struct{}) { close(serverDone) },
			finishLast:  func(pollsDone, _ chan struct{}) { close(pollsDone) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pollsDone := make(chan struct{})
			serverDone := make(chan struct{})
			transition := newProcessTransition(func() {}, pollsDone, serverDone)

			returned := make(chan struct{})
			go func() {
				transition.Wait()
				close(returned)
			}()

			tt.finishFirst(pollsDone, serverDone)
			select {
			case <-returned:
				t.Fatal("Wait returned before both poll schedules and server completed")
			case <-time.After(50 * time.Millisecond):
			}

			tt.finishLast(pollsDone, serverDone)
			select {
			case <-returned:
			case <-time.After(time.Second):
				t.Fatal("Wait did not return after poll schedules and server completed")
			}
		})
	}
}

func TestProcessTransitionReexecRequestsShutdownAndReturnsPathAfterCompletion(t *testing.T) {
	pollsDone := make(chan struct{})
	serverDone := make(chan struct{})
	stopRequested := make(chan struct{})
	transition := newProcessTransition(func() { close(stopRequested) }, pollsDone, serverDone)

	transition.RequestReexec("/opt/rfs/rfs")
	select {
	case <-stopRequested:
	case <-time.After(time.Second):
		t.Fatal("RequestReexec did not request shutdown")
	}

	type waitResult struct {
		path      string
		reexecute bool
	}
	returned := make(chan waitResult, 1)
	go func() {
		path, reexecute := transition.Wait()
		returned <- waitResult{path: path, reexecute: reexecute}
	}()

	close(pollsDone)
	close(serverDone)
	select {
	case result := <-returned:
		if !result.reexecute {
			t.Fatal("Wait did not report the requested re-execution")
		}
		if result.path != "/opt/rfs/rfs" {
			t.Fatalf("re-execution path = %q, want /opt/rfs/rfs", result.path)
		}
	case <-time.After(time.Second):
		t.Fatal("Wait did not return after completion")
	}
}
