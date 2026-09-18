package runtimecatalog

import "fmt"

type Release struct {
	Version     string
	ImageDigest string
	Adapter     string
	UID         int64
	GID         int64
}

var releases = map[string]Release{
	"v2026.9.14": {
		Version:     "v2026.9.14",
		ImageDigest: "sha256:99641e57ec762c59e54cb44aa6746b7fc68c18b3c5ddb088af54234c613d9294",
		Adapter:     "v20260914",
		UID:         10000,
		GID:         10000,
	},
}

func Resolve(version string) (Release, error) {
	r, ok := releases[version]
	if !ok {
		return Release{}, fmt.Errorf("unsupported Hermes release %q", version)
	}
	return r, nil
}
