package inspector

import "testing"

func TestFormatAge(t *testing.T) {
	tests := map[int64]string{-1: "?", 0: "0s", 45: "45s", 60: "1m", 8100: "2h15m", 273600: "3d4h"}
	for in, want := range tests {
		if got := FormatAge(in); got != want {
			t.Errorf("FormatAge(%d) = %q, want %q", in, got, want)
		}
	}
}
