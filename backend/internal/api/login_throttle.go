package api

import (
	"strings"
	"sync"
	"time"
)

// Login guessing budget. Ten misses in five minutes is far above what a human
// typo-ing a password produces, and far below what a password-guessing script
// needs to be useful.
const (
	loginMaxFailures = 10
	loginWindow      = 5 * time.Minute
	// Above this many tracked accounts a failure also sweeps expired entries,
	// so a script iterating usernames cannot grow the map without bound.
	loginSweepAt = 4096
)

// loginThrottle rate-limits failed logins per account.
//
// It keys on the *username*, not the client address: behind a reverse proxy
// every request arrives from the proxy's IP, so an address-keyed limiter would
// lock out the whole instance at once, and trusting X-Forwarded-For instead
// would let the attacker pick their own key. Username keying costs the ability
// for one user to be slowed by someone else's guessing — a nuisance next to
// unlimited attempts against `admin`.
type loginThrottle struct {
	mu     sync.Mutex
	fails  map[string]*failWindow
	max    int
	window time.Duration
	now    func() time.Time
}

type failWindow struct {
	count int
	start time.Time
}

func newLoginThrottle(max int, window time.Duration) *loginThrottle {
	return &loginThrottle{
		fails:  map[string]*failWindow{},
		max:    max,
		window: window,
		now:    time.Now,
	}
}

func throttleKey(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

// blocked reports whether the account is out of attempts, and for how long.
func (t *loginThrottle) blocked(key string) (time.Duration, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	w, ok := t.fails[key]
	if !ok {
		return 0, false
	}
	now := t.now()
	if now.Sub(w.start) >= t.window {
		delete(t.fails, key)
		return 0, false
	}
	if w.count < t.max {
		return 0, false
	}
	return t.window - now.Sub(w.start), true
}

// fail records a rejected attempt.
func (t *loginThrottle) fail(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	w, ok := t.fails[key]
	if !ok || now.Sub(w.start) >= t.window {
		t.fails[key] = &failWindow{count: 1, start: now}
	} else {
		w.count++
	}
	if len(t.fails) > loginSweepAt {
		for k, v := range t.fails {
			if now.Sub(v.start) >= t.window {
				delete(t.fails, k)
			}
		}
	}
}

// succeed clears the account's history — proving the password ends the penalty.
func (t *loginThrottle) succeed(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.fails, key)
}

// loginLimiter lazily builds the throttle: Server is constructed as a plain
// struct literal in main and in every test, so there is no constructor to hook.
func (s *Server) loginLimiter() *loginThrottle {
	s.loginOnce.Do(func() {
		s.loginThrottle = newLoginThrottle(loginMaxFailures, loginWindow)
	})
	return s.loginThrottle
}
