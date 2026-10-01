package handleproof

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDirectoryRejectsRedirectOversizeAndWrongStatus(t *testing.T) {
	const did = "did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"
	for _, entry := range []struct {
		name   string
		status int
		body   string
		valid  bool
	}{{"document", 200, `{"id":"` + did + `","alsoKnownAs":[],"service":[]}`, true}, {"oversize", 200, strings.Repeat(" ", 64*1024+1), false}, {"malformed", 200, `{`, false}, {"unavailable", 503, `{}`, false}, {"redirect", 302, `{}`, false}} {
		t.Run(entry.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/"+did {
					t.Error("unexpected directory path")
				}
				w.Header().Set("Location", "http://127.0.0.1:9/private")
				w.WriteHeader(entry.status)
				_, _ = w.Write([]byte(entry.body))
			}))
			defer server.Close()
			directory := NewDirectory(true)
			directory.base = server.URL
			doc, err := directory.Document(context.Background(), did)
			if (err == nil) != entry.valid {
				t.Fatal("directory response accepted incorrectly")
			}
			if err == nil && doc.ID != did {
				t.Fatal("document lost identity")
			}
		})
	}
	d := NewDirectory(true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	if _, err := d.Document(ctx, did); err == nil || time.Since(started) > time.Second {
		t.Fatal("directory ignored cancellation")
	}
	if _, err := d.Document(context.Background(), "../../private"); err == nil {
		t.Fatal("directory path injection")
	}
}
