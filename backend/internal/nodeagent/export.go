package nodeagent

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/base32"
	"errors"
	"os"
	"regexp"
	"strings"
	"time"
)

//go:embed export-helper.cjs
var exportHelperScript string
var portableCID = regexp.MustCompile(`^b[a-z2-7]{20,120}$`)
var publicSigningKey = regexp.MustCompile(`^did:key:z[1-9A-HJ-NP-Za-km-z]{20,120}$`)

// PortableBlob carries only referenced customer media, never native account settings.
type PortableBlob struct {
	CID      string `json:"cid"`
	MimeType string `json:"mimeType"`
	Data     []byte `json:"data"`
}

// PortableExport is private worker/agent transit content, not public deployment metadata.
type PortableExport struct {
	DID           string         `json:"did"`
	SigningKey    string         `json:"signingKey"`
	CAR           []byte         `json:"car"`
	Blobs         []PortableBlob `json:"blobs"`
	SignedRecords int            `json:"signedRecords"`
	CapturedAt    time.Time      `json:"capturedAt"`
}

// ValidFor checks fixed portable identifiers and strict local payload limits.
func (e PortableExport) ValidFor(did string) bool {
	if !accountDID.MatchString(did) || e.DID != did || !publicSigningKey.MatchString(e.SigningKey) || len(e.CAR) == 0 || len(e.CAR) > 8<<20 || len(e.Blobs) > 1000 || e.SignedRecords < 0 || e.SignedRecords > 10000 || e.CapturedAt.IsZero() || e.CapturedAt.After(time.Now().Add(5*time.Minute)) {
		return false
	}
	total := 0
	seen := map[string]bool{}
	for _, blob := range e.Blobs {
		if !portableCID.MatchString(blob.CID) || seen[blob.CID] || len(blob.MimeType) > 256 || len(blob.MimeType) == 0 || len(blob.Data) > 5<<20 {
			return false
		}
		hash := sha256.Sum256(blob.Data)
		expectedCID := "b" + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(append([]byte{1, 0x55, 0x12, 0x20}, hash[:]...)))
		if blob.CID != expectedCID {
			return false
		}
		seen[blob.CID] = true
		total += len(blob.Data)
	}
	return total <= 8<<20
}

// Export captures the sole expected actor through a pinned, read-only native helper.
func (d *Docker) Export(ctx context.Context, id, did string) (PortableExport, error) {
	if !d.localIdentity || !instanceID.MatchString(id) || !accountDID.MatchString(did) {
		return PortableExport{}, errors.New("fixed local portable export required")
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	current, err := d.status(ctx, id)
	if err != nil || current.State != "running" || !current.Healthy {
		return PortableExport{}, errors.New("healthy retained service required for export")
	}
	if lease, leaseErr := d.readAccountLease(id); leaseErr != nil && !errors.Is(leaseErr, os.ErrNotExist) {
		return PortableExport{}, errors.New("account export fence unavailable")
	} else if leaseErr == nil && (lease.Phase != "applied" || lease.DID != did) {
		return PortableExport{}, errors.New("confirmed account export fence required")
	}
	var result PortableExport
	if err = d.executePortable(ctx, "pds-ass-tenant-"+id, []string{"native_export", did}, &result); err != nil || !result.ValidFor(did) {
		return PortableExport{}, errors.New("portable native data could not be verified")
	}
	return result, nil
}
