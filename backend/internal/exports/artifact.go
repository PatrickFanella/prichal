// Package exports owns portable customer exports, independently of protected host recovery archives.
package exports

import (
	"archive/tar"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
)

const maxArchive = 20 << 20

var uuid = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

// File describes a portable archive entry, not a native host filename.
type File struct {
	Name     string `json:"name"`
	Bytes    int64  `json:"bytes"`
	SHA256   string `json:"sha256"`
	MimeType string `json:"mimeType,omitempty"`
}

// Manifest contains public signing identity and immutable exported-content hashes.
type Manifest struct {
	Version       int       `json:"version"`
	ExportID      string    `json:"exportId"`
	DeploymentID  string    `json:"deploymentId"`
	DID           string    `json:"did"`
	SigningKey    string    `json:"signingKey"`
	SignedRecords int       `json:"signedRecords"`
	CapturedAt    time.Time `json:"capturedAt"`
	Files         []File    `json:"files"`
}

// Artifacts encrypts private local files with scope-bound authenticated encryption.
type Artifacts struct {
	directory string
	aead      cipher.AEAD
}

// LoadArtifacts rejects public, symlinked or nonlocal configuration. Empty configuration disables exports.
func LoadArtifacts(local bool) (*Artifacts, error) {
	directory, key := os.Getenv("PDS_LOCAL_EXPORT_DIRECTORY"), os.Getenv("PDS_LOCAL_EXPORT_KEY_FILE")
	if directory == "" && key == "" {
		return nil, nil
	}
	if !local {
		return nil, errors.New("local export storage requires local mode")
	}
	return NewArtifacts(directory, key)
}

// NewArtifacts opens existing protected artifact custody. It creates no native resources.
func NewArtifacts(directory, keyPath string) (*Artifacts, error) {
	if !filepath.IsAbs(directory) {
		return nil, errors.New("absolute private export directory required")
	}
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil || resolved != filepath.Clean(directory) {
		return nil, errors.New("export directory must not traverse symlinks")
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("private export directory required")
	}
	info, err = os.Lstat(keyPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() != 32 {
		return nil, errors.New("private export key required")
	}
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, errors.New("export key unavailable")
	}
	if len(key) != 32 {
		return nil, errors.New("private AES-256 export key required")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Artifacts{directory, aead}, nil
}
func scope(id, deployment, did string) ([]byte, error) {
	if !uuid.MatchString(id) || !uuid.MatchString(deployment) || !regexp.MustCompile(`^did:plc:[a-z2-7]{24}$`).MatchString(did) {
		return nil, errors.New("invalid portable export scope")
	}
	return []byte("pds-portable-v1:" + id + ":" + deployment + ":" + did), nil
}
func digest(data []byte) string { hash := sha256.Sum256(data); return hex.EncodeToString(hash[:]) }
func archive(id, deployment, did string, data nodeagent.PortableExport) ([]byte, Manifest, error) {
	if _, err := scope(id, deployment, did); err != nil || !data.ValidFor(did) {
		return nil, Manifest{}, errors.New("portable content unavailable")
	}
	manifest := Manifest{Version: 1, ExportID: id, DeploymentID: deployment, DID: did, SigningKey: data.SigningKey, SignedRecords: data.SignedRecords, CapturedAt: data.CapturedAt, Files: []File{{Name: "repo.car", Bytes: int64(len(data.CAR)), SHA256: digest(data.CAR)}}}
	content := [][]byte{data.CAR}
	if len(data.Preferences) > 0 {
		manifest.Version = 2
		manifest.Files = append(manifest.Files, File{Name: "preferences.json", Bytes: int64(len(data.Preferences)), SHA256: digest(data.Preferences), MimeType: "application/json"})
		content = append(content, data.Preferences)
	}
	for _, blob := range data.Blobs {
		manifest.Files = append(manifest.Files, File{Name: "blobs/" + blob.CID, Bytes: int64(len(blob.Data)), SHA256: digest(blob.Data), MimeType: blob.MimeType})
		content = append(content, blob.Data)
	}
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	write := func(name string, data []byte) error {
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		_, err := writer.Write(data)
		return err
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return nil, Manifest{}, err
	}
	if err = write("manifest.json", encoded); err != nil {
		return nil, Manifest{}, err
	}
	for i, file := range manifest.Files {
		if err = write(file.Name, content[i]); err != nil {
			return nil, Manifest{}, err
		}
	}
	if err = writer.Close(); err != nil {
		return nil, Manifest{}, err
	}
	if buffer.Len() > maxArchive {
		return nil, Manifest{}, errors.New("portable archive exceeds local limit")
	}
	return buffer.Bytes(), manifest, nil
}

// Read authenticates the complete artifact before returning any customer content.
func (a *Artifacts) Read(id, deployment, did string) ([]byte, Manifest, error) {
	if a == nil {
		return nil, Manifest{}, errors.New("export custody unavailable")
	}
	aad, err := scope(id, deployment, did)
	if err != nil {
		return nil, Manifest{}, err
	}
	path := filepath.Join(a.directory, id+".sealed")
	info, err := os.Lstat(path)
	if err != nil {
		return nil, Manifest{}, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > maxArchive+64 {
		return nil, Manifest{}, errors.New("unsafe export artifact")
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		return nil, Manifest{}, err
	}
	size := a.aead.NonceSize()
	if len(encoded) < 1+size+a.aead.Overhead() || encoded[0] != 1 {
		return nil, Manifest{}, errors.New("invalid export envelope")
	}
	plain, err := a.aead.Open(nil, encoded[1:1+size], encoded[1+size:], aad)
	if err != nil {
		return nil, Manifest{}, errors.New("export artifact authentication failed")
	}
	reader := tar.NewReader(bytes.NewReader(plain))
	header, err := reader.Next()
	if err != nil || header.Name != "manifest.json" || header.Typeflag != tar.TypeReg || header.Size > 512<<10 {
		return nil, Manifest{}, errors.New("invalid export manifest")
	}
	var manifest Manifest
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&manifest) != nil || decoder.Decode(new(any)) != io.EOF || (manifest.Version != 1 && manifest.Version != 2) || manifest.ExportID != id || manifest.DeploymentID != deployment || manifest.DID != did || len(manifest.Files) < 1 || len(manifest.Files) > 1002 || manifest.Files[0].Name != "repo.car" || manifest.CapturedAt.IsZero() {
		return nil, Manifest{}, errors.New("invalid export manifest scope")
	}
	seen := map[string]bool{}
	for _, file := range manifest.Files {
		if seen[file.Name] || file.Bytes < 0 || file.Bytes > 8<<20 || file.Name != "repo.car" && file.Name != "preferences.json" && !regexp.MustCompile(`^blobs/b[a-z2-7]{20,120}$`).MatchString(file.Name) {
			return nil, Manifest{}, errors.New("invalid export member")
		}
		if file.Name == "preferences.json" && (manifest.Version != 2 || file.Bytes == 0 || file.Bytes > 1<<20 || file.MimeType != "application/json") {
			return nil, Manifest{}, errors.New("invalid private preference member")
		}
		seen[file.Name] = true
		header, err = reader.Next()
		if err != nil || header.Name != file.Name || header.Size != file.Bytes || header.Typeflag != tar.TypeReg {
			return nil, Manifest{}, errors.New("invalid export member")
		}
		data, err := io.ReadAll(io.LimitReader(reader, (8<<20)+1))
		if err != nil || int64(len(data)) != file.Bytes || digest(data) != file.SHA256 || file.Name == "preferences.json" && !nodeagent.ValidPreferences(data) {
			return nil, Manifest{}, errors.New("export member integrity failed")
		}
	}
	if manifest.Version == 2 && !seen["preferences.json"] {
		return nil, Manifest{}, errors.New("missing private preferences")
	}
	if _, err = reader.Next(); err != io.EOF {
		return nil, Manifest{}, errors.New("unexpected export members")
	}
	return plain, manifest, nil
}

// Publish preserves an existing authenticated artifact after lost-checkpoint replay.
func (a *Artifacts) Publish(id, deployment, did string, data nodeagent.PortableExport) ([]byte, Manifest, error) {
	if a == nil {
		return nil, Manifest{}, errors.New("export custody unavailable")
	}
	if plain, manifest, err := a.Read(id, deployment, did); err == nil {
		return plain, manifest, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, Manifest{}, err
	}
	plain, manifest, err := archive(id, deployment, did, data)
	if err != nil {
		return nil, Manifest{}, err
	}
	aad, err := scope(id, deployment, did)
	if err != nil {
		return nil, Manifest{}, err
	}
	nonce := make([]byte, a.aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, Manifest{}, err
	}
	encoded := append([]byte{1}, nonce...)
	encoded = append(encoded, a.aead.Seal(nil, nonce, plain, aad)...)
	temp, err := os.CreateTemp(a.directory, ".export-")
	if err != nil {
		return nil, Manifest{}, err
	}
	defer os.Remove(temp.Name())
	if _, err = temp.Write(encoded); err != nil {
		temp.Close()
		return nil, Manifest{}, err
	}
	if err = temp.Sync(); err != nil {
		temp.Close()
		return nil, Manifest{}, err
	}
	if err = temp.Close(); err != nil {
		return nil, Manifest{}, err
	}
	// A hard link publishes atomically without replacing another completed capture.
	if err = os.Link(temp.Name(), filepath.Join(a.directory, id+".sealed")); err != nil {
		if errors.Is(err, os.ErrExist) {
			return a.Read(id, deployment, did)
		}
		return nil, Manifest{}, err
	}
	directory, err := os.Open(a.directory)
	if err != nil {
		return nil, Manifest{}, err
	}
	err = directory.Sync()
	closeErr := directory.Close()
	if err != nil {
		return nil, Manifest{}, err
	}
	if closeErr != nil {
		return nil, Manifest{}, closeErr
	}
	return plain, manifest, nil
}
