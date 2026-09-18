package workload

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

//go:embed assets/*.py assets/adapters/*.py assets/SHA256SUMS
var runtimeAssets embed.FS

// RuntimeAssetsChecksum identifies the actual shipped script bytes, including
// adapters. It changes even if the catalog's adapter module name stays the same.
func RuntimeAssetsChecksum() string {
	return assetsChecksum(runtimeAssets)
}
func assetsChecksum(source fs.FS) string {
	data := map[string]string{}
	// The embedded filesystem cannot fail. Hash actual contents rather than
	// trusting the generated manifest: byte-only adapter changes must roll Pods.
	_ = fs.WalkDir(source, "assets", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			panic(err)
		}
		if !d.IsDir() && strings.HasSuffix(path, ".py") {
			content, err := fs.ReadFile(source, path)
			if err != nil {
				panic(err)
			}
			data[path] = string(content)
		}
		return nil
	})
	raw, _ := json.Marshal(data)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func bootstrapData(adapter string) (map[string]string, []corev1.KeyToPath, error) {
	if adapter != "v20260914" {
		return nil, nil, fmt.Errorf("unsupported runtime adapter %q", adapter)
	}
	data := map[string]string{}
	items := []corev1.KeyToPath{}
	err := fs.WalkDir(runtimeAssets, "assets", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".py") {
			return nil
		}
		b, err := runtimeAssets.ReadFile(path)
		if err != nil {
			return err
		}
		relative := strings.TrimPrefix(path, "assets/")
		key := strings.ReplaceAll(relative, "/", "__")
		data[key] = string(b)
		items = append(items, corev1.KeyToPath{Key: key, Path: relative})
		return nil
	})
	return data, items, err
}
