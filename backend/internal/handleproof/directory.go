package handleproof

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

// PLCDirectory uses an operator-selected fixed directory and bounded, redirect-free requests.
type PLCDirectory struct {
	base   string
	client *http.Client
}

func NewDirectory(local bool) *PLCDirectory {
	base := "https://plc.directory"
	if local {
		base = "http://localhost:2582"
	}
	return &PLCDirectory{base: base, client: &http.Client{Timeout: 4 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (d *PLCDirectory) Document(ctx context.Context, did string) (Document, error) {
	var doc Document
	if !plcDID.MatchString(did) {
		return doc, ErrInvalid
	}
	req, err := http.NewRequestWithContext(ctx, "GET", d.base+"/"+did, nil)
	if err != nil {
		return doc, err
	}
	response, err := d.client.Do(req)
	if err != nil {
		return doc, errors.New("identity directory unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return doc, errors.New("identity directory unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 64*1024+1))
	if err != nil || len(data) > 64*1024 {
		return doc, errors.New("identity document exceeds bound")
	}
	if json.Unmarshal(data, &doc) != nil {
		return Document{}, errors.New("invalid identity document")
	}
	return doc, nil
}
