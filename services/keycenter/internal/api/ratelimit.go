package api

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// UnlockRateLimiter tracks failed unlock attempts per IP and enforces
// cooldown periods with exponential backoff to prevent brute-force attacks.
type UnlockRateLimiter struct {
	mu          sync.Mutex
	attempts    map[string]*attemptRecord
	maxAttempts int
	baseCooldown time.Duration
	now         func() time.Time // injectable clock for testing
}

type attemptRecord struct {
	failures     int
	lockouts     int
	lockedUntil  time.Time
}

const (
	defaultMaxAttempts  = 5
	defaultBaseCooldown = 1 * time.Minute
	maxCooldown         = 15 * time.Minute
)

func NewUnlockRateLimiter() *UnlockRateLimiter {
	return &UnlockRateLimiter{
		attempts:     make(map[string]*attemptRecord),
		maxAttempts:  defaultMaxAttempts,
		baseCooldown: defaultBaseCooldown,
		now:          time.Now,
	}
}

func (rl *UnlockRateLimiter) cooldownFor(lockouts int) time.Duration {
	d := rl.baseCooldown
	for i := 1; i < lockouts; i++ {
		d *= 2
		if d > maxCooldown {
			return maxCooldown
		}
	}
	return d
}

// IsBlocked returns true and the remaining duration if the IP is currently locked out.
func (rl *UnlockRateLimiter) IsBlocked(ip string) (bool, time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rec, ok := rl.attempts[ip]
	if !ok {
		return false, 0
	}
	now := rl.now()
	if !rec.lockedUntil.IsZero() && now.Before(rec.lockedUntil) {
		return true, rec.lockedUntil.Sub(now)
	}
	return false, 0
}

// RecordFailure increments the failure count for an IP. When maxAttempts
// is reached, the IP enters a cooldown period with exponential backoff.
func (rl *UnlockRateLimiter) RecordFailure(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rec, ok := rl.attempts[ip]
	if !ok {
		rec = &attemptRecord{}
		rl.attempts[ip] = rec
	}

	// If a previous lockout has expired, reset failures but keep lockout count
	now := rl.now()
	if !rec.lockedUntil.IsZero() && now.After(rec.lockedUntil) {
		rec.failures = 0
		rec.lockedUntil = time.Time{}
	}

	rec.failures++
	if rec.failures >= rl.maxAttempts {
		rec.lockouts++
		cooldown := rl.cooldownFor(rec.lockouts)
		rec.lockedUntil = now.Add(cooldown)
		log.Printf("Rate limit: IP %s locked out for %v (lockout #%d)", ip, cooldown, rec.lockouts)
	}
}

// RecordSuccess clears all tracking for the given IP.
func (rl *UnlockRateLimiter) RecordSuccess(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.attempts, ip)
}

// Middleware wraps an http.HandlerFunc to enforce rate limiting.
// It uses a responseInterceptor to detect 401 (failure) vs 200 (success).
func (rl *UnlockRateLimiter) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clientIP, _, _ := net.SplitHostPort(r.RemoteAddr)
		if clientIP == "" {
			clientIP = r.RemoteAddr
		}

		if blocked, remaining := rl.IsBlocked(clientIP); blocked {
			log.Printf("Rate limit: blocked unlock attempt from %s (retry after %v)", clientIP, remaining)
			w.Header().Set("Retry-After", fmt.Sprintf("%d", int(remaining.Seconds())+1))
			respondRateLimited(w, remaining)
			return
		}

		rec := &responseInterceptor{ResponseWriter: w, statusCode: http.StatusOK}
		next(rec, r)

		switch rec.statusCode {
		case http.StatusUnauthorized:
			rl.RecordFailure(clientIP)
		case http.StatusOK:
			rl.RecordSuccess(clientIP)
		}
	}
}

func respondRateLimited(w http.ResponseWriter, remaining time.Duration) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	secs := int(remaining.Seconds()) + 1
	fmt.Fprintf(w, `{"error":"too many failed unlock attempts, retry after %d seconds"}`, secs)
	fmt.Fprintln(w)
}

// responseInterceptor captures the status code while passing through writes.
type responseInterceptor struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (ri *responseInterceptor) WriteHeader(code int) {
	ri.statusCode = code
	ri.ResponseWriter.WriteHeader(code)
	ri.written = true
}

func (ri *responseInterceptor) Write(b []byte) (int, error) {
	if !ri.written {
		ri.written = true
	}
	return ri.ResponseWriter.Write(b)
}
