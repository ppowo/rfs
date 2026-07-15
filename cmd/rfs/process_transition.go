package main

import "sync"

// processTransition coordinates shutdown completion with the decision to either
// exit normally or re-execute a newly installed binary. It does not own the
// poll schedules, HTTP server, or final exec effect.
type processTransition struct {
	requestStop func()
	pollsDone   <-chan struct{}
	serverDone  <-chan struct{}

	requestOnce sync.Once
	mu          sync.Mutex
	reexecute   bool
	reexecPath  string
}

func newProcessTransition(requestStop func(), pollsDone, serverDone <-chan struct{}) *processTransition {
	return &processTransition{
		requestStop: requestStop,
		pollsDone:   pollsDone,
		serverDone:  serverDone,
	}
}

func (t *processTransition) RequestReexec(path string) {
	t.requestOnce.Do(func() {
		t.mu.Lock()
		t.reexecute = true
		t.reexecPath = path
		t.mu.Unlock()
		t.requestStop()
	})
}

func (t *processTransition) Wait() (string, bool) {
	<-t.pollsDone
	<-t.serverDone

	t.mu.Lock()
	defer t.mu.Unlock()
	return t.reexecPath, t.reexecute
}
