package ratelimit

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	idleTimeout = 10 * time.Minute
	sweepInterval = time.Minute
)

type client struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type Limiter struct {
	mu         sync.Mutex
	clients    map[string]*client
	limit      rate.Limit
	burst      int
	retryAfter string
	lastSweep  time.Time
	now        func() time.Time
}

func New(requests int, window time.Duration) *Limiter {
	interval := window / time.Duration(requests)

	return &Limiter{
		clients:    map[string]*client{},
		limit:      rate.Every(interval),
		burst:      requests,
		retryAfter: strconv.Itoa(int(math.Ceil(interval.Seconds()))),
		now:        time.Now,
	}
}

func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(clientIP(r)) {
			w.Header().Set("Retry-After", l.retryAfter)
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (l *Limiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.removeIdleClients(now)

	c, ok := l.clients[ip]
	if !ok {
		c = &client{limiter: rate.NewLimiter(l.limit, l.burst)}
		l.clients[ip] = c
	}
	c.lastSeen = now

	return c.limiter.AllowN(now, 1)
}

func (l *Limiter) removeIdleClients(now time.Time) {
	if now.Sub(l.lastSweep) < sweepInterval {
		return
	}
	l.lastSweep = now

	for ip, c := range l.clients {
		if now.Sub(c.lastSeen) > idleTimeout {
			delete(l.clients, ip)
		}
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return host
}
