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

package ratelimit

import (
	"context"
	"sync"
	"time"
)

// Limiter implements a token bucket rate limiter
type Limiter struct {
	rate       float64       // tokens per second
	burst      int           // maximum bucket size
	tokens     float64       // current tokens
	lastUpdate time.Time     // last token update time
	mu         sync.Mutex
}

// NewLimiter creates a new rate limiter
// rate: requests per second
// burst: maximum burst size
func NewLimiter(rate float64, burst int) *Limiter {
	return &Limiter{
		rate:       rate,
		burst:      burst,
		tokens:     float64(burst),
		lastUpdate: time.Now(),
	}
}

// Allow checks if a request is allowed without blocking
func (l *Limiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.refill()

	if l.tokens >= 1 {
		l.tokens--
		return true
	}

	return false
}

// Wait blocks until a request is allowed or context is cancelled
func (l *Limiter) Wait(ctx context.Context) error {
	for {
		if l.Allow() {
			return nil
		}

		// Calculate wait time
		l.mu.Lock()
		waitTime := time.Duration(float64(time.Second) / l.rate)
		l.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
			// Try again
		}
	}
}

// Reserve reserves n tokens and returns the wait time
func (l *Limiter) Reserve(n int) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.refill()

	if l.tokens >= float64(n) {
		l.tokens -= float64(n)
		return 0
	}

	// Calculate wait time for needed tokens
	needed := float64(n) - l.tokens
	waitTime := time.Duration(needed / l.rate * float64(time.Second))

	// Reserve the tokens (go negative)
	l.tokens -= float64(n)

	return waitTime
}

// Tokens returns the current number of available tokens
func (l *Limiter) Tokens() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.refill()
	return l.tokens
}

// refill adds tokens based on elapsed time
func (l *Limiter) refill() {
	now := time.Now()
	elapsed := now.Sub(l.lastUpdate).Seconds()
	l.lastUpdate = now

	// Add tokens based on elapsed time
	l.tokens += elapsed * l.rate

	// Cap at burst
	if l.tokens > float64(l.burst) {
		l.tokens = float64(l.burst)
	}
}

// Registry manages rate limiters for multiple registries
type Registry struct {
	limiters map[string]*Limiter
	mu       sync.RWMutex

	// Default settings
	defaultRate  float64
	defaultBurst int
}

// NewRegistry creates a new limiter registry
func NewRegistry(defaultRate float64, defaultBurst int) *Registry {
	return &Registry{
		limiters:     make(map[string]*Limiter),
		defaultRate:  defaultRate,
		defaultBurst: defaultBurst,
	}
}

// Get returns the limiter for a registry, creating one if needed
func (r *Registry) Get(registryName string) *Limiter {
	r.mu.RLock()
	limiter, ok := r.limiters[registryName]
	r.mu.RUnlock()

	if ok {
		return limiter
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check
	if limiter, ok = r.limiters[registryName]; ok {
		return limiter
	}

	limiter = NewLimiter(r.defaultRate, r.defaultBurst)
	r.limiters[registryName] = limiter

	return limiter
}

// Set creates or updates a limiter with specific settings
func (r *Registry) Set(registryName string, rate float64, burst int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.limiters[registryName] = NewLimiter(rate, burst)
}

// Remove removes a limiter
func (r *Registry) Remove(registryName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.limiters, registryName)
}

// DefaultRegistry is the default limiter registry
var DefaultRegistry = NewRegistry(10, 20)

// GetLimiter returns a limiter for a registry using the default registry
func GetLimiter(registryName string) *Limiter {
	return DefaultRegistry.Get(registryName)
}
