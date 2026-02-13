package artifact

import (
	"testing"
)

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
