package handleproof

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
)

type dnsFixture struct {
	records []string
	err     error
}

func (d dnsFixture) LookupTXT(_ context.Context, name string) ([]string, error) {
	if d.err != nil {
		return nil, d.err
	}
	if name != "_atproto.creator.example.org" {
		return nil, errors.New("unexpected DNS name")
	}
	return d.records, d.err
}

type directoryFixture struct {
	doc Document
	err error
}

func (d directoryFixture) Document(context.Context, string) (Document, error) { return d.doc, d.err }
func TestNormalize(t *testing.T) {
	for _, entry := range []struct {
		input        string
		local, valid bool
	}{{"Creator.Example.org", false, true}, {"creator.test", true, true}, {"creator.test", false, false}, {"a.localhost", true, false}, {"a.invalid", true, false}, {"127.0.0.1", false, false}, {"http://example.org", false, false}, {"x..org", false, false}, {"x.org.", false, false}, {"x.0org", false, false}, {"-x.org", false, false}, {"user@x.org", false, false}, {"a" + strings.Repeat("b", 63) + ".org", false, false}, {"café.org", false, false}} {
		t.Run(entry.input, func(t *testing.T) {
			value, err := Normalize(entry.input, entry.local)
			if (err == nil) != entry.valid {
				t.Fatal("handle validation differs")
			}
			if err == nil && value != strings.ToLower(entry.input) {
				t.Fatal("case normalization failed")
			}
		})
	}
}
func TestBidirectionalProof(t *testing.T) {
	const did = "did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"
	const endpoint = "https://pds.example.org"
	base := Document{ID: did, AlsoKnownAs: []string{"at://creator.example.org"}, Service: []Service{{ID: "#atproto_pds", Type: "AtprotoPersonalDataServer", Endpoint: endpoint}}}
	for _, entry := range []struct {
		name, state string
		records     []string
		dnsErr      error
		doc         Document
		docErr      error
	}{
		{name: "verified", state: "verified", records: []string{"other=ignored", "did=" + did}, doc: base},
		{name: "duplicate same DID", state: "verified", records: []string{"did=" + did, "did=" + did}, doc: base},
		{name: "DNS delay", state: "pending_dns", dnsErr: &net.DNSError{IsNotFound: true}, doc: base},
		{name: "no proof", state: "pending_dns", records: []string{"other=ignored"}, doc: base},
		{name: "DNS failure", state: "unavailable", dnsErr: errors.New("PRIVATE DNS DETAILS"), doc: base},
		{name: "different DID", state: "conflict", records: []string{"did=did:plc:bbbbbbbbbbbbbbbbbbbbbbbb"}, doc: base},
		{name: "ambiguous", state: "conflict", records: []string{"did=" + did, "did=did:plc:bbbbbbbbbbbbbbbbbbbbbbbb"}, doc: base},
		{name: "invalid record", state: "conflict", records: []string{"did=not-a-did"}, doc: base},
		{name: "reverse absent", state: "pending_identity", records: []string{"did=" + did}, doc: Document{ID: did, Service: base.Service}},
		{name: "wrong document", state: "conflict", records: []string{"did=" + did}, doc: Document{ID: "did:plc:bbbbbbbbbbbbbbbbbbbbbbbb"}},
		{name: "wrong endpoint", state: "conflict", records: []string{"did=" + did}, doc: Document{ID: did, AlsoKnownAs: base.AlsoKnownAs, Service: []Service{{ID: "#atproto_pds", Type: "AtprotoPersonalDataServer", Endpoint: "https://other.example.org"}}}},
		{name: "duplicate service", state: "conflict", records: []string{"did=" + did}, doc: Document{ID: did, AlsoKnownAs: base.AlsoKnownAs, Service: append(base.Service, base.Service[0])}},
		{name: "directory outage", state: "unavailable", records: []string{"did=" + did}, docErr: errors.New("PRIVATE DIRECTORY DETAILS")},
	} {
		t.Run(entry.name, func(t *testing.T) {
			checker := Checker{DNS: dnsFixture{entry.records, entry.dnsErr}, Directory: directoryFixture{entry.doc, entry.docErr}}
			result := checker.Check(context.Background(), "creator.example.org", did, endpoint)
			if result.State != entry.state || strings.Contains(result.Message, "PRIVATE") {
				t.Fatalf("proof state: %s", result.State)
			}
		})
	}
}
