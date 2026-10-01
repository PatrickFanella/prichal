package exports

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
)

const exportID = "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa"
const deploymentID = "bbbbbbbb-bbbb-4bbb-bbbb-bbbbbbbbbbbb"
const fixtureDID = "did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"

func artifactFixture(t *testing.T) *Artifacts {
	t.Helper()
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "key")
	if err = os.WriteFile(path, key, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := NewArtifacts(directory, path)
	if err != nil {
		t.Fatal(err)
	}
	return store
}
func portableFixture() nodeagent.PortableExport {
	return nodeagent.PortableExport{DID: fixtureDID, SigningKey: "did:key:z222222222222222222222222222222222222222222222", CAR: []byte("fixture signed CAR"), SignedRecords: 2, CapturedAt: time.Now().UTC()}
}
func TestArtifactAuthenticationScopeAndImmutableReplay(t *testing.T) {
	store := artifactFixture(t)
	data := portableFixture()
	plain, manifest, err := store.Publish(exportID, deploymentID, fixtureDID, data)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ExportID != exportID || manifest.SignedRecords != 2 || !bytes.Contains(plain, []byte("repo.car")) {
		t.Fatal("portable manifest invalid")
	}
	encoded, err := os.ReadFile(filepath.Join(store.directory, exportID+".sealed"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, data.CAR) || bytes.Contains(encoded, []byte(fixtureDID)) {
		t.Fatal("artifact plaintext retained")
	}
	for _, scope := range []struct{ id, deployment, did string }{{exportID, "cccccccc-cccc-4ccc-cccc-cccccccccccc", fixtureDID}, {exportID, deploymentID, "did:plc:bbbbbbbbbbbbbbbbbbbbbbbb"}} {
		if leaked, _, err := store.Read(scope.id, scope.deployment, scope.did); err == nil || leaked != nil {
			t.Fatal("foreign artifact scope opened")
		}
	}
	data.CAR = []byte("later source version")
	data.CapturedAt = data.CapturedAt.Add(time.Minute)
	replay, replayed, err := store.Publish(exportID, deploymentID, fixtureDID, data)
	if err != nil || !bytes.Equal(replay, plain) || !replayed.CapturedAt.Equal(manifest.CapturedAt) {
		t.Fatal("lost checkpoint changed capture")
	}
	encoded[len(encoded)-1] ^= 1
	if err = os.WriteFile(filepath.Join(store.directory, exportID+".sealed"), encoded, 0600); err != nil {
		t.Fatal(err)
	}
	if leaked, _, err := store.Read(exportID, deploymentID, fixtureDID); err == nil || leaked != nil {
		t.Fatal("tampered content disclosed")
	}
	if _, _, err = store.Publish(exportID, deploymentID, fixtureDID, data); err == nil {
		t.Fatal("corrupt retained capture overwritten")
	}
}
func TestConcurrentArtifactPublicationRetainsOneCapture(t *testing.T) {
	store := artifactFixture(t)
	var wg sync.WaitGroup
	results := make(chan []byte, 8)
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			data := portableFixture()
			data.CAR = bytes.Repeat([]byte{byte(i + 1)}, 1024)
			plain, _, err := store.Publish(exportID, deploymentID, fixtureDID, data)
			if err != nil {
				failures <- err
			} else {
				results <- plain
			}
		}(i)
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	var original []byte
	for result := range results {
		if original == nil {
			original = result
		} else if !bytes.Equal(original, result) {
			t.Fatal("multiple captures published")
		}
	}
}
func TestArtifactPrivateCustodyAndWrongKey(t *testing.T) {
	store := artifactFixture(t)
	if _, _, err := store.Publish(exportID, deploymentID, fixtureDID, portableFixture()); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(store.directory, exportID+".sealed")
	if err := os.Chmod(file, 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Read(exportID, deploymentID, fixtureDID); err == nil {
		t.Fatal("public artifact accepted")
	}
	if err := os.Chmod(file, 0600); err != nil {
		t.Fatal(err)
	}
	wrong := filepath.Join(store.directory, "wrong-key")
	if err := os.WriteFile(wrong, bytes.Repeat([]byte{1}, 32), 0600); err != nil {
		t.Fatal(err)
	}
	other, err := NewArtifacts(store.directory, wrong)
	if err != nil {
		t.Fatal(err)
	}
	if plain, _, err := other.Read(exportID, deploymentID, fixtureDID); err == nil || plain != nil {
		t.Fatal("wrong key opened capture")
	}
	t.Setenv("PDS_LOCAL_EXPORT_DIRECTORY", store.directory)
	t.Setenv("PDS_LOCAL_EXPORT_KEY_FILE", wrong)
	if _, err := LoadArtifacts(false); err == nil {
		t.Fatal("live storage opened")
	}
}

func TestPrivatePreferenceArchivesPreserveVersionOneReplay(t *testing.T) {
	store := artifactFixture(t)
	data := portableFixture()
	old, _, err := store.Publish(exportID, deploymentID, fixtureDID, data)
	if err != nil {
		t.Fatal(err)
	}
	data.Preferences = []byte(`{"namespace":"app.bsky","preferences":[{"$type":"app.bsky.actor.defs#personalDetailsPref","birthDate":"2000-01-01"}]}`)
	replay, manifest, err := store.Publish(exportID, deploymentID, fixtureDID, data)
	if err != nil || manifest.Version != 1 || !bytes.Equal(old, replay) {
		t.Fatal("version one snapshot changed after preference support")
	}
	nextID := "dddddddd-dddd-4ddd-dddd-dddddddddddd"
	plain, manifest, err := store.Publish(nextID, deploymentID, fixtureDID, data)
	if err != nil || manifest.Version != 2 || len(manifest.Files) != 2 || manifest.Files[1].Name != "preferences.json" || !bytes.Contains(plain, data.Preferences) {
		t.Fatal("private preferences lost")
	}
	encoded, err := os.ReadFile(filepath.Join(store.directory, nextID+".sealed"))
	if err != nil || bytes.Contains(encoded, []byte("2000-01-01")) {
		t.Fatal("private preferences retained in plaintext")
	}
	reopened, _, err := store.Read(nextID, deploymentID, fixtureDID)
	if err != nil || !bytes.Equal(plain, reopened) {
		t.Fatal("version two preferences could not be authenticated")
	}
}
