/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package circuitbreaker

import (
	"errors"
	"sync"
	"time"
)

// State represents the circuit breaker state
type State int

const (
	// StateClosed allows requests through
	StateClosed State = iota
	// StateOpen blocks all requests
	StateOpen
	// StateHalfOpen allows limited requests to test recovery
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// ErrCircuitOpen is returned when the circuit is open
var ErrCircuitOpen = errors.New("circuit breaker is open")

// Breaker implements the circuit breaker pattern
type Breaker struct {
	name             string
	state            State
	failures         int
	successes        int
	lastFailure      time.Time
	lastStateChange  time.Time

	failureThreshold int
	successThreshold int
	timeout          time.Duration

	mu               sync.RWMutex
	onStateChange    func(name string, from, to State)
}

// Config holds circuit breaker configuration
type Config struct {
	// FailureThreshold is failures before opening
	FailureThreshold int
	// SuccessThreshold is successes before closing from half-open
	SuccessThreshold int
	// Timeout is time before transitioning from open to half-open
	Timeout time.Duration
	// OnStateChange is called when state changes
	OnStateChange func(name string, from, to State)
}

// DefaultConfig returns default configuration
func DefaultConfig() Config {
	return Config{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          5 * time.Minute,
	}
}

// NewBreaker creates a new circuit breaker
func NewBreaker(name string, cfg Config) *Breaker {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.SuccessThreshold <= 0 {
		cfg.SuccessThreshold = 2
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
	}

	return &Breaker{
		name:             name,
		state:            StateClosed,
		failureThreshold: cfg.FailureThreshold,
		successThreshold: cfg.SuccessThreshold,
		timeout:          cfg.Timeout,
		onStateChange:    cfg.OnStateChange,
		lastStateChange:  time.Now(),
	}
}

// Allow checks if a request should be allowed
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case StateClosed:
		return true

	case StateOpen:
		// Check if timeout has passed
		if time.Since(b.lastStateChange) > b.timeout {
			b.setState(StateHalfOpen)
			return true
		}
		return false

	case StateHalfOpen:
		// Allow limited requests in half-open state
		return true

	default:
		return false
	}
}

// RecordSuccess records a successful request
func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case StateClosed:
		// Reset failure count on success
		b.failures = 0

	case StateHalfOpen:
		b.successes++
		if b.successes >= b.successThreshold {
			b.setState(StateClosed)
		}
	}
}

// RecordFailure records a failed request
func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.lastFailure = time.Now()

	switch b.state {
	case StateClosed:
		b.failures++
		if b.failures >= b.failureThreshold {
			b.setState(StateOpen)
		}

	case StateHalfOpen:
		// Single failure in half-open goes back to open
		b.setState(StateOpen)
	}
}

// State returns the current state
func (b *Breaker) State() State {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.state
}

// Failures returns the current failure count
func (b *Breaker) Failures() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.failures
}

// LastFailure returns the time of the last failure
func (b *Breaker) LastFailure() time.Time {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.lastFailure
}

// Reset resets the circuit breaker to closed state
func (b *Breaker) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.failures = 0
	b.successes = 0
	b.setState(StateClosed)
}

// setState changes the state and calls the callback
func (b *Breaker) setState(newState State) {
	if b.state == newState {
		return
	}

	oldState := b.state
	b.state = newState
	b.lastStateChange = time.Now()
	b.successes = 0

	if b.onStateChange != nil {
		// Call callback without lock
		go b.onStateChange(b.name, oldState, newState)
	}
}

// Execute runs a function with circuit breaker protection
func (b *Breaker) Execute(fn func() error) error {
	if !b.Allow() {
		return ErrCircuitOpen
	}

	err := fn()

	if err != nil {
		b.RecordFailure()
	} else {
		b.RecordSuccess()
	}

	return err
}

// Registry manages circuit breakers for multiple registries
type Registry struct {
	breakers map[string]*Breaker
	mu       sync.RWMutex
	config   Config
}

// NewRegistry creates a new breaker registry
func NewRegistry(cfg Config) *Registry {
	return &Registry{
		breakers: make(map[string]*Breaker),
		config:   cfg,
	}
}

// Get returns the breaker for a registry, creating one if needed
func (r *Registry) Get(registryName string) *Breaker {
	r.mu.RLock()
	breaker, ok := r.breakers[registryName]
	r.mu.RUnlock()

	if ok {
		return breaker
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check
	if breaker, ok = r.breakers[registryName]; ok {
		return breaker
	}

	breaker = NewBreaker(registryName, r.config)
	r.breakers[registryName] = breaker

	return breaker
}

// Remove removes a breaker
func (r *Registry) Remove(registryName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.breakers, registryName)
}

// Reset resets all breakers
func (r *Registry) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, breaker := range r.breakers {
		breaker.Reset()
	}
}

// DefaultRegistry is the default breaker registry
var DefaultRegistry = NewRegistry(DefaultConfig())

// GetBreaker returns a breaker for a registry using the default registry
func GetBreaker(registryName string) *Breaker {
	return DefaultRegistry.Get(registryName)
}
