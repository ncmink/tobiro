package update

import "testing"

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.1", -1},
		{"v1.0.1", "1.0.0", 1},
		{"1.0.0", "1.0.0", 0},
		{"2.0.0", "1.9.9", 1},
	}
	for _, tc := range tests {
		if got := CompareVersions(tc.a, tc.b); got != tc.want {
			t.Fatalf("CompareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
