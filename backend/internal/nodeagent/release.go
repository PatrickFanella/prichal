package nodeagent

import (
	"context"
	"regexp"
)

var versionPattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:[-+][A-Za-z0-9.-]+)?$`)
var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
var pdsReference = regexp.MustCompile(`^ghcr\.io/bluesky-social/pds:([A-Za-z0-9_][A-Za-z0-9_.-]{0,127})@(sha256:[a-f0-9]{64})$`)

// RuntimeRelease binds the inspected container reference to its immutable image ID.
// Version is the inspected image OCI version label, when available.
type RuntimeRelease struct {
	Version  string `json:"version"`
	ImageRef string `json:"imageRef"`
	Digest   string `json:"digest"`
	ImageID  string `json:"imageId"`
	Tag      string `json:"tag"`
}

func (r RuntimeRelease) Valid() bool {
	parts := pdsReference.FindStringSubmatch(r.ImageRef)
	return len(parts) == 3 && parts[1] == r.Tag && parts[2] == r.Digest && digestPattern.MatchString(r.ImageID) && (r.Version == "" || (len(r.Version) <= 128 && versionPattern.MatchString(r.Version)))
}

func (d *Docker) runtimeRelease(ctx context.Context, current inspected) *RuntimeRelease {
	parts := pdsReference.FindStringSubmatch(current.Config.Image)
	if !current.State.Running || len(parts) != 3 || !digestPattern.MatchString(current.Image) {
		return nil
	}
	var image struct {
		ID          string
		RepoDigests []string
		Config      struct{ Labels map[string]string }
	}
	if _, err := d.call(ctx, "GET", "/images/"+current.Image+"/json", nil, &image); err != nil || image.ID != current.Image {
		return nil
	}
	// Docker RepoDigests omits the tag; the digest is a manifest/index digest,
	// distinct from the image configuration ID in container.Image.
	for _, digest := range image.RepoDigests {
		if digest == "ghcr.io/bluesky-social/pds@"+parts[2] {
			version := image.Config.Labels["org.opencontainers.image.version"]
			if len(version) > 128 || !versionPattern.MatchString(version) {
				version = ""
			}
			return &RuntimeRelease{Version: version, ImageRef: current.Config.Image, Digest: parts[2], ImageID: current.Image, Tag: parts[1]}
		}
	}
	return nil
}

// ParseRuntimeRelease is used only when reading the bounded management projection.
func ParseRuntimeRelease(reference, imageID, version string) *RuntimeRelease {
	parts := pdsReference.FindStringSubmatch(reference)
	if len(parts) != 3 {
		return nil
	}
	r := &RuntimeRelease{Version: version, ImageRef: reference, Digest: parts[2], ImageID: imageID, Tag: parts[1]}
	if !r.Valid() {
		return nil
	}
	return r
}
