package pccc

import (
	"strings"
	"testing"
)

// ===========================================================================
// ParseAddress — happy path
//
// SLC syntax:
//   <type><file>:<element>[.<field> | /<bit>]
// Plus the special status form S:<element> (file number is implicitly 2).
// ===========================================================================

func TestParseAddressBasic(t *testing.T) {
	tests := []struct {
		in       string
		ft       FileType
		fileNum  int
		elem     int
		subElem  int
		bit      int
	}{
		// Plain word reads — no sub-field, no bit.
		{"N7:0", FileTypeInteger, 7, 0, 0, -1},
		{"N7:42", FileTypeInteger, 7, 42, 0, -1},
		{"F8:5", FileTypeFloat, 8, 5, 0, -1},
		{"B3:0", FileTypeBit, 3, 0, 0, -1},
		// O and I have implicit file numbers (O=0, I=1) just like S=2.
		{"O:0", FileTypeOutput, 0, 0, 0, -1},
		{"I:1", FileTypeInput, 1, 1, 0, -1},
		// Two-letter mnemonic.
		{"ST10:0", FileTypeString, 10, 0, 0, -1},
		// Status file — file number is fixed at 2 in SLC, even though the
		// syntax has no explicit file number.
		{"S:1", FileTypeStatus, 2, 1, 0, -1},
		{"S:7", FileTypeStatus, 2, 7, 0, -1},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			a, err := ParseAddress(tc.in)
			if err != nil {
				t.Fatalf("ParseAddress(%q): %v", tc.in, err)
			}
			if a.FileType != tc.ft {
				t.Errorf("FileType: got 0x%02X want 0x%02X", byte(a.FileType), byte(tc.ft))
			}
			if a.FileNumber != tc.fileNum {
				t.Errorf("FileNumber: got %d want %d", a.FileNumber, tc.fileNum)
			}
			if a.Element != tc.elem {
				t.Errorf("Element: got %d want %d", a.Element, tc.elem)
			}
			if a.SubElement != tc.subElem {
				t.Errorf("SubElement: got %d want %d", a.SubElement, tc.subElem)
			}
			if a.BitNum != tc.bit {
				t.Errorf("BitNum: got %d want %d", a.BitNum, tc.bit)
			}
		})
	}
}

func TestParseAddressBitSuffix(t *testing.T) {
	tests := []struct {
		in  string
		bit int
	}{
		{"B3:0/0", 0},
		{"B3:0/15", 15},
		{"N7:5/2", 2},
		{"N7:5/7", 7},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			a, err := ParseAddress(tc.in)
			if err != nil {
				t.Fatalf("ParseAddress(%q): %v", tc.in, err)
			}
			if a.BitNum != tc.bit {
				t.Errorf("BitNum: got %d want %d", a.BitNum, tc.bit)
			}
			if a.SubElement != 0 {
				t.Errorf("SubElement should be 0 for /bit form, got %d", a.SubElement)
			}
		})
	}
}

// Timer fields:
//   .PRE -> subelement 1
//   .ACC -> subelement 2
//   .EN  -> subelement 0, bit 15
//   .TT  -> subelement 0, bit 14
//   .DN  -> subelement 0, bit 13
func TestParseAddressTimerFields(t *testing.T) {
	tests := []struct {
		in     string
		sub    int
		bit    int
	}{
		{"T4:0.PRE", 1, -1},
		{"T4:0.ACC", 2, -1},
		{"T4:0.EN", 0, 15},
		{"T4:0.TT", 0, 14},
		{"T4:0.DN", 0, 13},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			a, err := ParseAddress(tc.in)
			if err != nil {
				t.Fatalf("ParseAddress(%q): %v", tc.in, err)
			}
			if a.FileType != FileTypeTimer {
				t.Errorf("FileType: got %s want Timer", a.FileType)
			}
			if a.SubElement != tc.sub {
				t.Errorf("SubElement: got %d want %d", a.SubElement, tc.sub)
			}
			if a.BitNum != tc.bit {
				t.Errorf("BitNum: got %d want %d", a.BitNum, tc.bit)
			}
		})
	}
}

// Counter fields:
//   .PRE -> subelement 1
//   .ACC -> subelement 2
//   .CU/.CD/.DN/.OV/.UN -> subelement 0, bits 15/14/13/12/11
func TestParseAddressCounterFields(t *testing.T) {
	tests := []struct {
		in  string
		sub int
		bit int
	}{
		{"C5:0.PRE", 1, -1},
		{"C5:0.ACC", 2, -1},
		{"C5:0.CU", 0, 15},
		{"C5:0.CD", 0, 14},
		{"C5:0.DN", 0, 13},
		{"C5:0.OV", 0, 12},
		{"C5:0.UN", 0, 11},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			a, err := ParseAddress(tc.in)
			if err != nil {
				t.Fatalf("ParseAddress(%q): %v", tc.in, err)
			}
			if a.FileType != FileTypeCounter {
				t.Errorf("FileType: got %s want Counter", a.FileType)
			}
			if a.SubElement != tc.sub {
				t.Errorf("SubElement: got %d want %d", a.SubElement, tc.sub)
			}
			if a.BitNum != tc.bit {
				t.Errorf("BitNum: got %d want %d", a.BitNum, tc.bit)
			}
		})
	}
}

// Control (R) fields:
//   .LEN -> subelement 1
//   .POS -> subelement 2
//   control bits live in subelement 0
func TestParseAddressControlFields(t *testing.T) {
	tests := []struct {
		in  string
		sub int
		bit int
	}{
		{"R6:0.LEN", 1, -1},
		{"R6:0.POS", 2, -1},
		{"R6:0.EN", 0, 15},
		{"R6:0.DN", 0, 13},
		{"R6:0.ER", 0, 11},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			a, err := ParseAddress(tc.in)
			if err != nil {
				t.Fatalf("ParseAddress(%q): %v", tc.in, err)
			}
			if a.FileType != FileTypeControl {
				t.Errorf("FileType: got %s want Control", a.FileType)
			}
			if a.SubElement != tc.sub {
				t.Errorf("SubElement: got %d want %d", a.SubElement, tc.sub)
			}
			if a.BitNum != tc.bit {
				t.Errorf("BitNum: got %d want %d", a.BitNum, tc.bit)
			}
		})
	}
}

func TestParseAddressCaseInsensitive(t *testing.T) {
	a1, err := ParseAddress("n7:0")
	if err != nil {
		t.Fatalf("ParseAddress(n7:0): %v", err)
	}
	a2, err := ParseAddress("N7:0")
	if err != nil {
		t.Fatalf("ParseAddress(N7:0): %v", err)
	}
	if a1 != a2 {
		t.Errorf("case-insensitive parse mismatch:\n got %+v\nwant %+v", a1, a2)
	}

	// Field names are case-insensitive too.
	a3, err := ParseAddress("t4:0.acc")
	if err != nil {
		t.Fatalf("ParseAddress(t4:0.acc): %v", err)
	}
	if a3.SubElement != 2 {
		t.Errorf("lower-case .acc: SubElement got %d want 2", a3.SubElement)
	}
}

func TestParseAddressTrimsWhitespace(t *testing.T) {
	a, err := ParseAddress("  N7:0  ")
	if err != nil {
		t.Fatalf("ParseAddress: %v", err)
	}
	if a.FileType != FileTypeInteger || a.FileNumber != 7 || a.Element != 0 {
		t.Errorf("got %+v", a)
	}
}

func TestParseAddressString(t *testing.T) {
	// Address.String should round-trip back to a canonical form. For the
	// simple cases it should equal the upper-case input.
	cases := []string{
		"N7:0",
		"F8:5",
		"B3:0/2",
		"T4:0.ACC",
		"T4:0.EN",
		"C5:0.PRE",
		"S:1",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			a, err := ParseAddress(in)
			if err != nil {
				t.Fatalf("ParseAddress(%q): %v", in, err)
			}
			if got := a.String(); got != in {
				t.Errorf("String(): got %q want %q", got, in)
			}
		})
	}
}

// ===========================================================================
// Malformed input
// ===========================================================================

func TestParseAddressErrors(t *testing.T) {
	tests := []struct {
		in       string
		mustHave string // substring expected in the error message
	}{
		{"", "empty"},
		{"   ", "empty"},
		{"N", "missing"},
		{"N7", "missing"},
		{"N7:", "missing"},
		{":0", "file type"},
		{"X7:0", "file type"},
		{"N-1:0", "file"},
		{"N7:-1", "element"},
		{"N7:0/16", "bit"},
		{"N7:0/-1", "bit"},
		{"N7:0.XYZ", "field"},
		{"T4:0.FOO", "field"},
		// Bit suffix illegal on float file (whole element is 32 bits, but
		// SLC bit addressing applies only to 16-bit word files).
		{"F8:0/0", "bit"},
		// Trailing garbage.
		{"N7:0abc", "trailing"},
		{"N7:0/2/3", "trailing"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			_, err := ParseAddress(tc.in)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", tc.in)
			}
			if tc.mustHave != "" && !strings.Contains(strings.ToLower(err.Error()), tc.mustHave) {
				t.Errorf("error %q should contain %q", err.Error(), tc.mustHave)
			}
		})
	}
}

// Status file is special: writing the file number explicitly should still
// work (and must equal 2), and any other value must be rejected.
func TestParseAddressStatusFileNumber(t *testing.T) {
	a, err := ParseAddress("S2:1")
	if err != nil {
		t.Fatalf("ParseAddress(S2:1): %v", err)
	}
	if a.FileNumber != 2 {
		t.Errorf("S2:1 FileNumber: got %d want 2", a.FileNumber)
	}

	if _, err := ParseAddress("S5:1"); err == nil {
		t.Errorf("S5:1 should be rejected (status file is always 2)")
	}
}
