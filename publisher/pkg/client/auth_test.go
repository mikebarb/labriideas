// pkg/client/auth_test.go
package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestDoReauthenticatesOn401(t *testing.T) {
	var logins, protected int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			atomic.AddInt32(&logins, 1)
			json.NewEncoder(w).Encode(map[string]string{
				"token": "fresh-token", "userID": "dev",
				"expiresAt": "2099-01-01T00:00:00Z",
			})
		case "/api/update-metadata":
			// Reject anything but the renewed token — i.e. simulate a server
			// that restarted and forgot the old session.
			if r.Header.Get("Authorization") != "Bearer fresh-token" {
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}
			atomic.AddInt32(&protected, 1)
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, apiToken: "stale-session", password: "pw", ownsSession: true}
	if err := c.UpdateMetadata("x.mp3", map[string]string{"title": "t"}); err != nil {
		t.Fatalf("expected transparent re-auth, got %v", err)
	}
	if logins != 1 {
		t.Fatalf("expected exactly 1 login, got %d", logins)
	}
	if protected != 1 {
		t.Fatalf("expected the retried call to succeed once, got %d", protected)
	}
}
