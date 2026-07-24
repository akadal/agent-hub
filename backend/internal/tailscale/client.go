// Package tailscale lists tailnet devices via the public Tailscale API.
// Used for one-shot machine import — no daemon required on the API host.
package tailscale

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.tailscale.com"

// Client talks to api.tailscale.com with an API access token.
type Client struct {
	APIKey  string
	Tailnet string // use "-" for the key owner's default tailnet
	HTTP    *http.Client
	BaseURL string
}

// Device is a minimal view of a tailnet node for import.
type Device struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`       // MagicDNS FQDN when available
	Hostname   string   `json:"hostname"`
	OS         string   `json:"os"`
	Addresses  []string `json:"addresses"`
	Authorized bool     `json:"authorized"`
	LastSeen   time.Time `json:"last_seen"`
	Online     bool     `json:"online"`
	// PreferredAddress is the best SSH target (IPv4 100.x preferred).
	PreferredAddress string `json:"preferred_address"`
}

type devicesResponse struct {
	Devices []apiDevice `json:"devices"`
}

type apiDevice struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Hostname   string   `json:"hostname"`
	OS         string   `json:"os"`
	Addresses  []string `json:"addresses"`
	Authorized bool     `json:"authorized"`
	LastSeen   string   `json:"lastSeen"`
}

// Configured reports whether an API key is present.
func (c *Client) Configured() bool {
	return c != nil && strings.TrimSpace(c.APIKey) != ""
}

// ListDevices returns authorized devices with a usable address.
func (c *Client) ListDevices(ctx context.Context) ([]Device, error) {
	if !c.Configured() {
		return nil, fmt.Errorf("TAILSCALE_API_KEY is not set")
	}
	tailnet := strings.TrimSpace(c.Tailnet)
	if tailnet == "" {
		tailnet = "-"
	}
	base := c.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	url := fmt.Sprintf("%s/api/v2/tailnet/%s/devices", strings.TrimRight(base, "/"), tailnet)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// Tailscale accepts Basic (key:) or Bearer.
	req.SetBasicAuth(c.APIKey, "")
	req.Header.Set("Accept", "application/json")

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tailscale api: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		var body struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(res.Body).Decode(&body)
		msg := body.Message
		if msg == "" {
			msg = res.Status
		}
		return nil, fmt.Errorf("tailscale api %d: %s", res.StatusCode, msg)
	}

	var payload devicesResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode devices: %w", err)
	}

	now := time.Now().UTC()
	out := make([]Device, 0, len(payload.Devices))
	for _, d := range payload.Devices {
		if !d.Authorized {
			continue
		}
		addr := preferredAddress(d.Addresses)
		if addr == "" {
			continue
		}
		lastSeen, _ := time.Parse(time.RFC3339, d.LastSeen)
		// Heuristic: seen in the last 10 minutes ≈ online.
		online := !lastSeen.IsZero() && now.Sub(lastSeen) < 10*time.Minute
		name := strings.TrimSpace(d.Hostname)
		if name == "" {
			name = strings.TrimSpace(d.Name)
		}
		if name == "" {
			name = addr
		}
		// Prefer short hostname for display; strip MagicDNS suffix noise later in UI.
		out = append(out, Device{
			ID:               d.ID,
			Name:             name,
			Hostname:         d.Hostname,
			OS:               d.OS,
			Addresses:        d.Addresses,
			Authorized:       d.Authorized,
			LastSeen:         lastSeen,
			Online:           online,
			PreferredAddress: addr,
		})
	}
	return out, nil
}

// preferredAddress picks the best SSH target: Tailscale IPv4 first, else first IPv4, else first addr.
func preferredAddress(addrs []string) string {
	var firstIPv4, firstAny string
	for _, a := range addrs {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if firstAny == "" {
			firstAny = a
		}
		ip := net.ParseIP(a)
		if ip == nil {
			continue
		}
		ip4 := ip.To4()
		if ip4 == nil {
			continue
		}
		if firstIPv4 == "" {
			firstIPv4 = a
		}
		// CGNAT 100.64/10 is Tailscale's usual range
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return a
		}
	}
	if firstIPv4 != "" {
		return firstIPv4
	}
	return firstAny
}
