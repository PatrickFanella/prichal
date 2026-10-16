package support

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"sync"
	"time"
)

const upstreamURL = "https://api.github.com/repos/bluesky-social/pds/releases/latest"

type upstreamRelease struct {
	State     string     `json:"state"`
	Tag       string     `json:"tag"`
	CheckedAt *time.Time `json:"checkedAt"`
}

// One cache per API process, shared by all operators. Page requests never wait
// for upstream; failures discard the tag and are cached for five minutes.
type upstreamCache struct {
	mu     sync.Mutex
	client *http.Client
	now    func() time.Time
	value  upstreamRelease
	next   time.Time
	busy   bool
}

func newUpstreamCache() *upstreamCache {
	return &upstreamCache{client: &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("upstream redirects refused") }}, now: time.Now, value: upstreamRelease{State: "unknown"}}
}

func (c *upstreamCache) snapshot() upstreamRelease {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.busy && !c.now().Before(c.next) {
		c.busy = true
		c.value.State, c.value.Tag = "unknown", ""
		go c.refresh()
	}
	return c.value
}

var releaseTag = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:[-+][A-Za-z0-9.-]+)?$`)

func (c *upstreamCache) refresh() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, "GET", upstreamURL, nil)
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "Prichal-release-observer")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	value := upstreamRelease{State: "unknown"}
	response, err := c.client.Do(request)
	if err == nil {
		defer response.Body.Close()
		if response.StatusCode == 200 {
			var body struct {
				Tag        string `json:"tag_name"`
				Draft      bool   `json:"draft"`
				Prerelease bool   `json:"prerelease"`
			}
			payload, readErr := io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
			if readErr == nil && len(payload) <= 64<<10 && json.Unmarshal(payload, &body) == nil && !body.Draft && !body.Prerelease && len(body.Tag) <= 128 && releaseTag.MatchString(body.Tag) {
				value.State, value.Tag = "observed", body.Tag
			}
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now().UTC()
	value.CheckedAt = &now
	c.value = value
	c.next = now.Add(5 * time.Minute)
	if value.State == "observed" {
		c.next = now.Add(time.Hour)
	}
	c.busy = false
}
