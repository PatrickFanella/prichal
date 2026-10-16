package support

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type releaseTransport func(*http.Request) (*http.Response, error)

func (f releaseTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func waitRelease(t *testing.T, c *upstreamCache) upstreamRelease {
	t.Helper()
	deadline := time.After(time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		c.mu.Lock()
		busy, value := c.busy, c.value
		c.mu.Unlock()
		if !busy {
			return value
		}
		select {
		case <-deadline:
			t.Fatal("cache did not finish")
		case <-tick.C:
		}
	}
}
func TestUpstreamLookupDoesNotBlockAndSharesCache(t *testing.T) {
	c := newUpstreamCache()
	now := time.Now()
	c.now = func() time.Time { return now }
	var calls atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	c.client.Transport = releaseTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != upstreamURL || r.Method != "GET" || r.Header.Get("Authorization") != "" {
			t.Error("lookup authority changed")
		}
		calls.Add(1)
		close(entered)
		<-release
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"tag_name":"v0.4.999","draft":false,"prerelease":false}`)), Header: make(http.Header)}, nil
	})
	if value := c.snapshot(); value.State != "unknown" || value.CheckedAt != nil {
		t.Fatal(value)
	}
	<-entered
	for i := 0; i < 20; i++ {
		if c.snapshot().State != "unknown" {
			t.Fatal("pending lookup guessed a release")
		}
	}
	close(release)
	value := waitRelease(t, c)
	if value.State != "observed" || value.Tag != "v0.4.999" || value.CheckedAt == nil {
		t.Fatal(value)
	}
	for i := 0; i < 20; i++ {
		c.snapshot()
	}
	if calls.Load() != 1 {
		t.Fatal("cache missed")
	}
	c.mu.Lock()
	now = now.Add(time.Hour + time.Second)
	c.client.Transport = releaseTransport(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("PRIVATE upstream error")
	})
	c.mu.Unlock()
	if c.snapshot().State != "unknown" {
		t.Fatal("expired cache claimed current release")
	}
	value = waitRelease(t, c)
	if value.State != "unknown" || value.Tag != "" || value.CheckedAt == nil {
		t.Fatal(value)
	}
	for i := 0; i < 20; i++ {
		c.snapshot()
	}
	if calls.Load() != 2 {
		t.Fatal("failure not cached")
	}
}
func TestUpstreamFailuresAreUnknown(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"no published releases", `{}`, 404}, {"rate limited", `{}`, 403}, {"malformed", `{`, 200}, {"draft", `{"tag_name":"v1.2.3","draft":true}`, 200}, {"prerelease", `{"tag_name":"v1.2.3","prerelease":true}`, 200}, {"unsafe tag", `{"tag_name":"https://example.com"}`, 200}, {"oversized", strings.Repeat(" ", 64<<10) + `{"tag_name":"v1.2.3"}`, 200}, {"trailing", `{"tag_name":"v1.2.3"}{}`, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newUpstreamCache()
			c.client.Transport = releaseTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})
			c.snapshot()
			value := waitRelease(t, c)
			if value.State != "unknown" || value.Tag != "" || value.CheckedAt == nil {
				t.Fatal(value)
			}
		})
	}
	c := newUpstreamCache()
	c.client.Transport = releaseTransport(func(r *http.Request) (*http.Response, error) {
		if deadline, ok := r.Context().Deadline(); !ok || time.Until(deadline) > 3*time.Second {
			t.Error("unbounded lookup")
		}
		return nil, errors.New("unreachable")
	})
	c.snapshot()
	if value := waitRelease(t, c); value.State != "unknown" || value.CheckedAt == nil {
		t.Fatal(value)
	}
}
