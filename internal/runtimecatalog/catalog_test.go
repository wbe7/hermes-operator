package runtimecatalog

import "testing"

func TestUnknownReleaseIsRejected(t *testing.T) {
	if _, err := Resolve("latest"); err == nil {
		t.Fatal("mutable or unsupported release accepted")
	}
}

func TestVerifiedRelease(t *testing.T) {
	r, err := Resolve("v2026.9.14")
	if err != nil {
		t.Fatal(err)
	}
	if r.ImageDigest != "sha256:feecb5d71f4876758b61e83cf1527fa6cdbb4f7e8919808c5eed7daf5085640f" || r.UpstreamImageDigest != "sha256:99641e57ec762c59e54cb44aa6746b7fc68c18b3c5ddb088af54234c613d9294" || r.Adapter != "v20260914" || r.UID != 10000 || r.GID != 10000 {
		t.Fatalf("unexpected release: %#v", r)
	}
}

func TestImageSelectionPreservesOriginalInstallationsAndMirrors(t *testing.T) {
	r := Release{ImageDigest: "document-digest", UpstreamImageDigest: "upstream-digest"}
	for _, test := range []struct{ repository, digest, wantRepository, wantDigest string }{
		{"", "", "docker.io/wbe7/hermes", "document-digest"},
		{"docker.io/wbe7/hermes", "", "docker.io/wbe7/hermes", "document-digest"},
		{"docker.io/nousresearch/hermes-agent", "", "docker.io/nousresearch/hermes-agent", "upstream-digest"},
		{"nousresearch/hermes-agent", "", "nousresearch/hermes-agent", "upstream-digest"},
		{"mirror.example/hermes", "upstream-digest", "mirror.example/hermes", "upstream-digest"},
		{"mirror.example/hermes", "", "mirror.example/hermes", "document-digest"},
	} {
		repository, digest := r.ResolveImage(test.repository, test.digest)
		if repository != test.wantRepository || digest != test.wantDigest {
			t.Fatalf("%#v: got %s@%s", test, repository, digest)
		}
	}
}
