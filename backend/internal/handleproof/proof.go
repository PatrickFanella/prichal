// Package handleproof checks domain claims without changing native accounts or PDS resources.
package handleproof

import (
	"context"
	"errors"
	"net"
	"regexp"
	"strings"
)

var syntax = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`)
var plcDID = regexp.MustCompile(`^did:plc:[a-z2-7]{24}$`)
var ErrInvalid = errors.New("invalid customer handle")

// Normalize accepts protocol hostname syntax and rejects reserved namespaces.
func Normalize(value string, local bool) (string, error) {
	value = strings.ToLower(value)
	if len(value) > 244 || !syntax.MatchString(value) {
		return "", ErrInvalid
	}
	labels := strings.Split(value, ".")
	switch labels[len(labels)-1] {
	case "alt", "arpa", "example", "internal", "invalid", "local", "localhost", "onion":
		return "", ErrInvalid
	case "test":
		if !local {
			return "", ErrInvalid
		}
	}
	return value, nil
}

// TXTResolver supports the protocol's preferred DNS proof without fetching customer URLs.
type TXTResolver interface {
	LookupTXT(context.Context, string) ([]string, error)
}

// Directory resolves a fixed, trusted PLC directory, never a customer-provided URL.
type Directory interface {
	Document(context.Context, string) (Document, error)
}
type Document struct {
	ID          string    `json:"id"`
	AlsoKnownAs []string  `json:"alsoKnownAs"`
	Service     []Service `json:"service"`
}
type Service struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Endpoint string `json:"serviceEndpoint"`
}
type Result struct {
	State   string `json:"state"`
	Message string `json:"message"`
}
type Checker struct {
	DNS       TXTResolver
	Directory Directory
	Local     bool
}

// Check verifies a unique forward claim and the reverse alias and stable hosting endpoint.
func (c Checker) Check(ctx context.Context, handle, did, endpoint string) Result {
	result := func(state, message string) Result { return Result{state, message} }
	normalized, err := Normalize(handle, c.Local)
	if err != nil || normalized != handle || !plcDID.MatchString(did) || c.DNS == nil || c.Directory == nil {
		return result("invalid", "Use a valid domain handle and confirmed account.")
	}
	records, err := c.DNS.LookupTXT(ctx, "_atproto."+handle)
	if err != nil {
		var dns *net.DNSError
		if errors.As(err, &dns) && dns.IsNotFound {
			return result("pending_dns", "Publish the TXT record, then check again after DNS propagates.")
		}
		return result("unavailable", "Domain proof could not be checked. Retry later.")
	}
	if len(records) > 64 {
		return result("conflict", "The DNS answer contains too many records.")
	}
	unique := map[string]bool{}
	for _, record := range records {
		if strings.HasPrefix(record, "did=") {
			value := strings.TrimPrefix(record, "did=")
			if !plcDID.MatchString(value) {
				return result("conflict", "A domain proof record contains an invalid DID.")
			}
			unique[value] = true
		}
	}
	if len(unique) == 0 {
		return result("pending_dns", "Publish the TXT record, then check again after DNS propagates.")
	}
	if len(unique) != 1 || !unique[did] {
		return result("conflict", "The domain's TXT records conflict with this account DID.")
	}
	doc, err := c.Directory.Document(ctx, did)
	if err != nil {
		return result("unavailable", "The account identity could not be checked. Retry later.")
	}
	if doc.ID != did {
		return result("conflict", "The identity document does not match this account DID.")
	}
	alias := false
	for _, value := range doc.AlsoKnownAs {
		if value == "at://"+handle {
			alias = true
		}
	}
	if !alias {
		return result("pending_identity", "DNS matches. Sign in to your native account and change its handle, then check again.")
	}
	count := 0
	correct := false
	for _, service := range doc.Service {
		if service.ID == "#atproto_pds" || service.ID == did+"#atproto_pds" {
			count++
			correct = service.Type == "AtprotoPersonalDataServer" && service.Endpoint == endpoint
		}
	}
	if count != 1 || !correct {
		return result("conflict", "The identity document's PDS endpoint differs from this deployment.")
	}
	return result("verified", "DNS and account identity both confirm this handle. Your PDS endpoint is unchanged.")
}
