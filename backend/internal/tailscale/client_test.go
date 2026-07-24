package tailscale

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPreferredAddress(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"fd7a:115c:a1e0::1", "100.64.1.5"}, "100.64.1.5"},
		{[]string{"10.0.0.1", "100.100.50.1"}, "100.100.50.1"},
		{[]string{"192.168.1.1"}, "192.168.1.1"},
		{[]string{"fd7a:115c:a1e0::1"}, "fd7a:115c:a1e0::1"},
		{nil, ""},
	}
	for _, tc := range cases {
		got := preferredAddress(tc.in)
		if got != tc.want {
			t.Errorf("preferredAddress(%v)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestListDevices_filtersAndMaps(t *testing.T) {
	now := time.Now().UTC()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/tailnet/-/devices" {
			t.Errorf("path=%s", r.URL.Path)
		}
		user, _, ok := r.BasicAuth()
		if !ok || user != "tskey-test" {
			http.Error(w, "auth", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"devices": []map[string]any{
				{
					"id":         "n1",
					"name":       "box.tailnet.ts.net",
					"hostname":   "box",
					"os":         "linux",
					"addresses":  []string{"100.64.0.2"},
					"authorized": true,
					"lastSeen":   now.Format(time.RFC3339),
				},
				{
					"id":         "n2",
					"name":       "phone",
					"hostname":   "phone",
					"os":         "iOS",
					"addresses":  []string{"100.64.0.3"},
					"authorized": true,
					"lastSeen":   now.Add(-2 * time.Hour).Format(time.RFC3339),
				},
				{
					"id":         "n3",
					"hostname":   "unauth",
					"addresses":  []string{"100.64.0.4"},
					"authorized": false,
					"lastSeen":   now.Format(time.RFC3339),
				},
			},
		})
	}))
	defer srv.Close()

	c := &Client{APIKey: "tskey-test", Tailnet: "-", BaseURL: srv.URL}
	devs, err := c.ListDevices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 2 {
		t.Fatalf("got %d devices want 2", len(devs))
	}
	if devs[0].Name != "box" || devs[0].PreferredAddress != "100.64.0.2" || !devs[0].Online {
		t.Fatalf("dev0=%+v", devs[0])
	}
	if devs[1].Online {
		t.Fatalf("phone should be offline heuristic")
	}
}

func TestListDevices_notConfigured(t *testing.T) {
	c := &Client{}
	_, err := c.ListDevices(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}
