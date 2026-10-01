package nodeagent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func exportFixture() PortableExport {
	data := []byte("referenced media")
	hash := sha256.Sum256(data)
	cid := "b" + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(append([]byte{1, 0x55, 0x12, 0x20}, hash[:]...)))
	return PortableExport{DID: "did:plc:aaaaaaaaaaaaaaaaaaaaaaaa", SigningKey: "did:key:z222222222222222222222222222222222222222222222", CAR: []byte("fixture CAR"), Blobs: []PortableBlob{{CID: cid, MimeType: "image/png", Data: data}}, CapturedAt: time.Now()}
}

type exportRuntime struct {
	fakeRuntime
	calls int
}

func (f *exportRuntime) Export(context.Context, string, string) (PortableExport, error) {
	f.calls++
	return exportFixture(), nil
}
func TestPortableExportIdentifiersContentAndBounds(t *testing.T) {
	valid := exportFixture()
	if !valid.ValidFor(valid.DID) {
		t.Fatal("valid portable shape rejected")
	}
	bad := valid
	bad.Blobs = append([]PortableBlob{}, valid.Blobs...)
	bad.Blobs[0].Data = []byte("altered media")
	if bad.ValidFor(valid.DID) {
		t.Fatal("media hash mismatch accepted")
	}
	if valid.ValidFor("did:plc:bbbbbbbbbbbbbbbbbbbbbbbb") {
		t.Fatal("foreign DID accepted")
	}
	bad = valid
	bad.CAR = make([]byte, (8<<20)+1)
	if bad.ValidFor(valid.DID) {
		t.Fatal("unbounded CAR")
	}
}
func TestFixedPortableExportHTTPContract(t *testing.T) {
	runtime := &exportRuntime{}
	handler := Handler(runtime)
	id := "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa"
	did := exportFixture().DID
	for _, tc := range []struct {
		body   string
		status int
	}{{`{"did":"` + did + `"}`, 200}, {`{"did":"` + did + `","path":"/pds/keys"}`, 400}, {`{"did":"../../keys"}`, 400}, {`{"did":"` + did + `"} {}`, 400}} {
		request := httptest.NewRequest("POST", "/v1/instances/"+id+"/export", strings.NewReader(tc.body))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != tc.status {
			t.Fatalf("status %d", response.Code)
		}
		if tc.status == 200 {
			var result PortableExport
			if json.Unmarshal(response.Body.Bytes(), &result) != nil || !result.ValidFor(did) {
				t.Fatal("invalid transit response")
			}
		}
	}
	if runtime.calls != 1 {
		t.Fatal("invalid requests reached capture")
	}
}
func TestPortableFrameLimitsDoNotWidenOrdinaryHelpers(t *testing.T) {
	frame := helperFrame(1, bytes.Repeat([]byte{'a'}, 64000))
	if _, err := decodeHelperOutput(bytes.NewReader(frame)); err == nil {
		t.Fatal("ordinary helper limit widened")
	}
	if output, err := decodeBoundedHelperOutput(bytes.NewReader(frame), 24<<20, 32<<20, 1<<20); err != nil || len(output) != 64000 {
		t.Fatal("bounded portable frame rejected")
	}
	if output, err := decodeBoundedHelperOutput(bytes.NewReader(helperFrame(2, []byte("PRIVATE CUSTOMER DATA"))), 24<<20, 32<<20, 1<<20); err == nil || output != nil {
		t.Fatal("stderr disclosed")
	}
}
