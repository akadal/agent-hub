package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/akadal/agent-hub/backend/internal/api"
	"github.com/akadal/agent-hub/backend/internal/auth"
	"github.com/akadal/agent-hub/backend/internal/store"
)

// The published demo password makes `admin` the obvious guessing target, so the
// login route must stop answering after a burst of misses.
func TestLoginStopsAnsweringAfterRepeatedFailures(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureBootstrapAdmin("admin", "correct-horse"); err != nil {
		t.Fatal(err)
	}
	mux := (&api.Server{Store: st, Tokens: auth.NewTokenService("test-secret", time.Hour)}).NewMux()

	attempt := func(user, pass string) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		_ = json.NewEncoder(&buf).Encode(map[string]string{"username": user, "password": pass})
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest("POST", "/api/auth/login", &buf))
		return rr
	}

	var blockedAt int
	for i := 1; i <= 30; i++ {
		rr := attempt("admin", "wrong")
		if rr.Code == http.StatusTooManyRequests {
			blockedAt = i
			if rr.Header().Get("Retry-After") == "" {
				t.Error("429 without a Retry-After header")
			}
			break
		}
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status %d, want 401", i, rr.Code)
		}
	}
	if blockedAt == 0 {
		t.Fatal("30 wrong passwords in a row and the endpoint still answers")
	}

	// While blocked, even the correct password is refused — otherwise the
	// throttle would be a free oracle for "is this the right one?".
	if rr := attempt("admin", "correct-horse"); rr.Code != http.StatusTooManyRequests {
		t.Fatalf("correct password while blocked: status %d, want 429", rr.Code)
	}

	// A different account is not collateral damage.
	if rr := attempt("nobody", "whatever"); rr.Code != http.StatusUnauthorized {
		t.Fatalf("unrelated account: status %d, want 401", rr.Code)
	}
}
