package rfs

import (
	"context"
	"sync"
	"time"
)

type DomainGateConfig struct {
	MinSpacing     time.Duration
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

type GateClock interface {
	Now() time.Time
	Sleep(context.Context, time.Duration) error
}

type DomainGate struct {
	mu     sync.Mutex
	clock  GateClock
	config DomainGateConfig
	states map[string]*domainGateState
}

type domainGateState struct {
	lock        sync.Mutex
	nextAllowed time.Time
	backoff     time.Duration
}

func NewDomainGate(clock GateClock, config DomainGateConfig) *DomainGate {
	if clock == nil {
		clock = realGateClock{}
	}
	if config.InitialBackoff == 0 {
		config.InitialBackoff = time.Minute
	}
	if config.MaxBackoff == 0 {
		config.MaxBackoff = time.Hour
	}
	return &DomainGate{
		clock:  clock,
		config: config,
		states: map[string]*domainGateState{},
	}
}

// Run waits for the domain gate, runs action while holding that domain's lane,
// and records success/throttle state from the PollResult. Calls for different
// domains can run independently; calls for the same domain are serialized.
func (g *DomainGate) Run(ctx context.Context, domain string, action func() (PollResult, error)) (PollResult, error) {
	state := g.stateFor(domain)
	state.lock.Lock()
	defer state.lock.Unlock()

	if err := g.waitLocked(ctx, state); err != nil {
		return PollResult{}, err
	}

	result, err := action()
	if err != nil {
		g.recordThrottleLocked(state, 0)
		return result, err
	}
	if result.Status == PollThrottled {
		g.recordThrottleLocked(state, result.RetryAfter)
		return result, nil
	}
	g.recordSuccessLocked(state)
	return result, nil
}

func (g *DomainGate) waitLocked(ctx context.Context, state *domainGateState) error {
	for {
		delay := state.nextAllowed.Sub(g.clock.Now())
		if delay <= 0 {
			return nil
		}
		if err := g.clock.Sleep(ctx, delay); err != nil {
			return err
		}
	}
}

func (g *DomainGate) recordSuccessLocked(state *domainGateState) {
	state.backoff = 0
	state.nextAllowed = g.clock.Now().Add(g.config.MinSpacing)
}

func (g *DomainGate) recordThrottleLocked(state *domainGateState, retryAfter time.Duration) {
	delay := retryAfter
	if delay <= 0 {
		delay = g.nextBackoff(state.backoff)
	}
	state.backoff = delay
	nextAllowed := g.clock.Now().Add(delay)
	if nextAllowed.After(state.nextAllowed) {
		state.nextAllowed = nextAllowed
	}
}

func (g *DomainGate) stateFor(domain string) *domainGateState {
	g.mu.Lock()
	defer g.mu.Unlock()

	state, ok := g.states[domain]
	if !ok {
		state = &domainGateState{}
		g.states[domain] = state
	}
	return state
}

func (g *DomainGate) nextBackoff(previous time.Duration) time.Duration {
	if previous <= 0 {
		return g.config.InitialBackoff
	}
	next := previous * 2
	if next > g.config.MaxBackoff {
		return g.config.MaxBackoff
	}
	return next
}

type realGateClock struct{}

func (realGateClock) Now() time.Time { return time.Now().UTC() }

func (realGateClock) Sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
