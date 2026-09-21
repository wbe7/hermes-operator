package runtimecatalog

import (
	"fmt"
	"strings"
)

const DefaultImageRepository = "docker.io/wbe7/hermes"

type Release struct {
	Version             string
	ImageDigest         string
	UpstreamImageDigest string
	Adapter             string
	UID                 int64
	GID                 int64
}

var releases = map[string]Release{
	"v2026.9.14": {
		Version:             "v2026.9.14",
		ImageDigest:         "sha256:feecb5d71f4876758b61e83cf1527fa6cdbb4f7e8919808c5eed7daf5085640f",
		UpstreamImageDigest: "sha256:99641e57ec762c59e54cb44aa6746b7fc68c18b3c5ddb088af54234c613d9294",
		Adapter:             "v20260914",
		UID:                 10000,
		GID:                 10000,
	},
}

// ResolveImage preserves existing CRs whose defaulted repository names the
// official runtime. Other repositories mirror the document runtime by default;
// an explicit, supported digest also allows mirrors of the original image.
func (r Release) ResolveImage(repository, digest string) (string, string) {
	if repository == "" {
		repository = DefaultImageRepository
	}
	if digest == "" {
		digest = r.ImageDigest
		name := strings.TrimPrefix(strings.TrimPrefix(repository, "docker.io/"), "index.docker.io/")
		if name == "nousresearch/hermes-agent" {
			digest = r.UpstreamImageDigest
		}
	}
	return repository, digest
}

func Resolve(version string) (Release, error) {
	r, ok := releases[version]
	if !ok {
		return Release{}, fmt.Errorf("unsupported Hermes release %q", version)
	}
	return r, nil
}
