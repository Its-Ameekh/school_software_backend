// Package middleware contains Gin middleware for the School Software
// backend. This file implements Stage 3 item 18: an in-memory,
// per-IP rate limiter for authenticated resource endpoints (e.g.
// /api/dashboard/stats, /api/finance/summary).
//
// Design recap (see roadmap Stage 3, item 18):
//   - This does NOT protect login. Login (signInWithPassword) never
//     reaches this backend at all — it goes straight to Supabase. This
//     middleware only throttles hammering of *our* endpoints once a
//     caller already has a token.
//   - Keyed by client IP (c.ClientIP()), not by user ID, so it also
//     covers pre-auth / malformed-token requests hitting protected
//     routes, not just already-authenticated callers.
//   - Deliberately in-memory (no Redis) to keep the single-Lightsail-
//     instance footprint simple. NOTE: this means limits are per-process.
//     If this backend is ever scaled to multiple instances behind a
//     load balancer, this in-memory approach stops being correct (each
//     instance would enforce its own separate limit) and should be
//     swapped for a shared store (e.g. Redis) at that point.
package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Rate limiter parameters. 60 requests/minute per IP, with a small burst
// allowance on top so a legitimate dashboard page loading several
// widgets at once doesn't get falsely throttled.
const (
	rateLimitWindow   = time.Minute
	rateLimitMax      = 60 // requests allowed per window
	rateLimitBurst    = 10 // extra requests allowed on top of rateLimitMax, absorbed instantly
	rateLimitCapacity = rateLimitMax + rateLimitBurst

	// cleanupInterval controls how often stale IP buckets are purged so
	// the in-memory map doesn't grow unbounded over the life of the
	// process.
	cleanupInterval = 10 * time.Minute
	staleAfter      = 10 * time.Minute
)

// visitor tracks one IP's token bucket state.
type visitor struct {
	tokens     float64
	lastSeen   time.Time
	lastRefill time.Time
}

// RateLimiter is a thread-safe, in-memory, per-IP token-bucket limiter.
// Build one with NewRateLimiter and mount its Limit() method on whatever
// route group needs protecting. Do not construct one per request.
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
}

// NewRateLimiter builds a ready-to-use RateLimiter and starts its
// background cleanup goroutine. Call this once at startup (e.g. in
// main.go) and reuse the same instance across route groups if they
// should share one limit, or construct separate instances if different
// route groups need different ceilings.
func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
	}
	go rl.cleanupLoop()
	return rl
}

// Limit returns the Gin middleware itself. Mount it on any route group
// that needs protection, e.g.:
//
//	limiter := middleware.NewRateLimiter()
//	protected := router.Group("/api")
//	protected.Use(authMW.RequireAuth(), limiter.Limit())
func (rl *RateLimiter) Limit() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()

		if !rl.allow(ip) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":  "too_many_requests",
				"detail": "rate limit exceeded, please slow down and try again shortly",
			})
			return
		}

		c.Next()
	}
}

// allow implements a token-bucket check-and-consume for the given IP,
// refilling tokens continuously based on elapsed time since the bucket's
// last refill. Returns true if the request is allowed (and consumes one
// token), false if the bucket is empty.
func (rl *RateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	v, exists := rl.visitors[ip]
	if !exists {
		// New visitor starts with a full bucket minus the token this
		// request consumes.
		rl.visitors[ip] = &visitor{
			tokens:     rateLimitCapacity - 1,
			lastSeen:   now,
			lastRefill: now,
		}
		return true
	}

	// Refill tokens continuously: rateLimitMax tokens per rateLimitWindow
	// duration, capped at rateLimitCapacity.
	elapsed := now.Sub(v.lastRefill)
	refillRate := float64(rateLimitMax) / rateLimitWindow.Seconds() // tokens per second
	v.tokens += elapsed.Seconds() * refillRate
	if v.tokens > rateLimitCapacity {
		v.tokens = rateLimitCapacity
	}
	v.lastRefill = now
	v.lastSeen = now

	if v.tokens < 1 {
		return false
	}

	v.tokens--
	return true
}

// cleanupLoop periodically purges visitors that haven't made a request
// in a while, so the map doesn't grow unbounded for the lifetime of the
// process. Runs for as long as the process runs — there is deliberately
// no stop channel here, since a RateLimiter is expected to live for the
// full process lifetime (same pattern as AuthMiddleware's JWKS refresh).
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		cutoff := time.Now().Add(-staleAfter)
		rl.mu.Lock()
		for ip, v := range rl.visitors {
			if v.lastSeen.Before(cutoff) {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}
