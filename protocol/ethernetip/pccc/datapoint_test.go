package pccc

import (
	"testing"

	"github.com/iceisfun/goindustrial/plc"
)

func TestFileImplementsDataPoint(t *testing.T) {
	var _ plc.DataPoint = File{Address: "N7:0"}
}

func TestFileString(t *testing.T) {
	tests := []struct {
		f    File
		want string
	}{
		{File{Address: "N7:0"}, "File(N7:0)"},
		{File{Address: "N7:0", Count: 1}, "File(N7:0)"},
		{File{Address: "N7:0", Count: 5}, "File(N7:0, count=5)"},
		{File{Address: "T4:0.ACC"}, "File(T4:0.ACC)"},
		{File{}, "File()"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.f.String(); got != tc.want {
				t.Errorf("String(): got %q want %q", got, tc.want)
			}
		})
	}
}

func TestFileEffectiveCount(t *testing.T) {
	tests := []struct {
		in   int
		want int
	}{
		{0, 1}, {-1, 1}, {1, 1}, {2, 2}, {10, 10},
	}
	for _, tc := range tests {
		f := File{Count: tc.in}
		if got := f.effectiveCount(); got != tc.want {
			t.Errorf("effectiveCount(%d): got %d want %d", tc.in, got, tc.want)
		}
	}
}
