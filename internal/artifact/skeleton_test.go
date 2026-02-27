package artifact

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGoldenVariantsSameSkeleton verifies A4: golden dataset variants (different timestamps/addresses)
// produce the same skeleton_hash. Run from repo root: go test ./internal/artifact -run TestGoldenVariantsSameSkeleton
func TestGoldenVariantsSameSkeleton(t *testing.T) {
	pairs := []struct {
		file1, file2 string
	}{
		{"testdata/golden/traceback/04_pytest_fail.txt", "testdata/golden/traceback/04_pytest_fail_variant.txt"},
		{"testdata/golden/ci/01_github_actions.log", "testdata/golden/ci/01_github_actions_variant.log"},
	}
	for _, p := range pairs {
		b1, err := os.ReadFile(p.file1)
		if err != nil {
			t.Skipf("golden file not found: %s", p.file1)
			return
		}
		b2, err := os.ReadFile(p.file2)
		if err != nil {
			t.Skipf("golden file not found: %s", p.file2)
			return
		}
		h1 := SkeletonHash(string(b1))
		h2 := SkeletonHash(string(b2))
		if h1 != h2 {
			t.Errorf("A4 skeleton stability: %s and %s should have same hash, got %s != %s",
				filepath.Base(p.file1), filepath.Base(p.file2), h1, h2)
		}
	}
}

func TestSkeletonHash(t *testing.T) {
	a := "error at 2024-02-12T10:30:00 in pid 12345 addr 0x7f8b"
	b := "error at 2024-02-13T11:00:00 in pid 99999 addr 0xabcd"
	ha := SkeletonHash(a)
	hb := SkeletonHash(b)
	if ha != hb {
		t.Errorf("same skeleton structure should match: %s != %s", ha, hb)
	}
}

func TestSkeletonHashDifferent(t *testing.T) {
	a := "error: file not found"
	b := "error: permission denied"
	ha := SkeletonHash(a)
	hb := SkeletonHash(b)
	if ha == hb {
		t.Errorf("different content should differ: %s == %s", ha, hb)
	}
}
