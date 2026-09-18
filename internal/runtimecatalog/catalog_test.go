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
	if r.ImageDigest != "sha256:99641e57ec762c59e54cb44aa6746b7fc68c18b3c5ddb088af54234c613d9294" || r.Adapter != "v20260914" || r.UID != 10000 || r.GID != 10000 {
		t.Fatalf("unexpected release: %#v", r)
	}
}
