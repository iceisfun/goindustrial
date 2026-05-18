package pccc

import (
	"fmt"
	"strconv"
	"strings"
)

// Address is a parsed SLC data-table reference such as N7:0, B3:0/2,
// F8:5, or T4:0.ACC. It carries everything the framing layer needs to
// build a typed-logical PCCC request, plus an optional bit number for
// callers that want to read or write a single bit.
type Address struct {
	// FileType is the SLC file type code (e.g. FileTypeInteger for N files).
	FileType FileType

	// FileNumber is the file number from the address (e.g. 7 in N7:0).
	// For S addresses without an explicit file number, this is 2.
	FileNumber int

	// Element is the element index within the file (the integer after ':').
	Element int

	// SubElement selects a 16-bit word within a multi-word element. For
	// scalar files (N, F, B, S, ...) it is always 0. For timer/counter/
	// control files it is 0 for the control word, 1 for PRE/LEN, and 2 for
	// ACC/POS.
	SubElement int

	// BitNum is the bit position within the addressed word, or -1 if no
	// bit suffix or named bit field was used.
	BitNum int
}

// String returns a canonical upper-case form of the address. It is the
// inverse of [ParseAddress] for well-formed inputs.
func (a Address) String() string {
	var sb strings.Builder
	if def, ok := implicitFileNumber(a.FileType); ok && a.FileNumber == def {
		// Omit the file number for S, I, O when it equals the default.
		sb.WriteString(a.FileType.Letter())
		sb.WriteByte(':')
		sb.WriteString(strconv.Itoa(a.Element))
	} else {
		sb.WriteString(a.FileType.Letter())
		sb.WriteString(strconv.Itoa(a.FileNumber))
		sb.WriteByte(':')
		sb.WriteString(strconv.Itoa(a.Element))
	}
	// Prefer the named field form when we recognise the (subElem, bit)
	// pair for a timer/counter/control file.
	if name, ok := fieldNameFor(a.FileType, a.SubElement, a.BitNum); ok {
		sb.WriteByte('.')
		sb.WriteString(name)
		return sb.String()
	}
	if a.BitNum >= 0 {
		sb.WriteByte('/')
		sb.WriteString(strconv.Itoa(a.BitNum))
	}
	return sb.String()
}

// ParseAddress parses an SLC data-table address such as "N7:0", "B3:0/2",
// "F8:5", "T4:0.ACC", or "S:1". The parser is case-insensitive and ignores
// surrounding whitespace.
func ParseAddress(s string) (Address, error) {
	in := strings.TrimSpace(s)
	if in == "" {
		return Address{}, fmt.Errorf("pccc: empty address")
	}
	up := strings.ToUpper(in)

	// 1. Split off the optional .FIELD or /BIT suffix.
	mainPart, suffix, sep := splitSuffix(up)

	// 2. Split main into <letters><digits>:<element>. The letters identify
	//    the file type; the digits are the file number (absent for S).
	left, right, ok := strings.Cut(mainPart, ":")
	if !ok {
		return Address{}, fmt.Errorf("pccc: missing ':' in %q", s)
	}
	if right == "" {
		return Address{}, fmt.Errorf("pccc: missing element number in %q", s)
	}

	letters, fileNumStr := splitLeadingLetters(left)
	if letters == "" {
		return Address{}, fmt.Errorf("pccc: missing file type in %q", s)
	}
	ft, ok := FileTypeFromLetter(letters)
	if !ok {
		return Address{}, fmt.Errorf("pccc: unknown file type %q in %q", letters, s)
	}

	// 3. File number. S/I/O have implicit defaults (2/1/0); other file
	//    types require an explicit file number. An explicit number that
	//    conflicts with the implicit default is rejected.
	var fileNum int
	if fileNumStr == "" {
		def, ok := implicitFileNumber(ft)
		if !ok {
			return Address{}, fmt.Errorf("pccc: missing file number in %q", s)
		}
		fileNum = def
	} else {
		if !isAllDigits(fileNumStr) {
			return Address{}, fmt.Errorf("pccc: invalid file number %q in %q", fileNumStr, s)
		}
		n, err := strconv.Atoi(fileNumStr)
		if err != nil {
			return Address{}, fmt.Errorf("pccc: invalid file number %q in %q", fileNumStr, s)
		}
		if n < 0 || n > 255 {
			return Address{}, fmt.Errorf("pccc: file number %d out of range [0,255] in %q", n, s)
		}
		if def, ok := implicitFileNumber(ft); ok && n != def {
			return Address{}, fmt.Errorf("pccc: file type %s requires file number %d (got %d) in %q",
				ft.Letter(), def, n, s)
		}
		fileNum = n
	}

	// 4. Element — must be a plain integer; trailing non-digit garbage
	//    is reported explicitly.
	if !isAllDigitsOrLeadingSign(right) {
		return Address{}, fmt.Errorf("pccc: trailing garbage after element in %q", s)
	}
	elem, err := strconv.Atoi(right)
	if err != nil {
		return Address{}, fmt.Errorf("pccc: invalid element %q in %q", right, s)
	}
	if elem < 0 || elem > 255 {
		return Address{}, fmt.Errorf("pccc: element %d out of range [0,255] in %q", elem, s)
	}

	addr := Address{
		FileType:   ft,
		FileNumber: fileNum,
		Element:    elem,
		BitNum:     -1,
	}

	// 5. Suffix: either ".FIELD" (named timer/counter/control field) or
	//    "/N" (bit number 0..15 within the word).
	switch sep {
	case 0:
		// no suffix
	case '.':
		sub, bit, ok := lookupNamedField(ft, suffix)
		if !ok {
			return Address{}, fmt.Errorf("pccc: unknown field %q for file type %s in %q",
				suffix, ft.Letter(), s)
		}
		addr.SubElement = sub
		addr.BitNum = bit
	case '/':
		if strings.ContainsAny(suffix, "./") {
			return Address{}, fmt.Errorf("pccc: trailing garbage after bit in %q", s)
		}
		if !isAllDigitsOrLeadingSign(suffix) {
			return Address{}, fmt.Errorf("pccc: invalid bit %q in %q", suffix, s)
		}
		bit, err := strconv.Atoi(suffix)
		if err != nil {
			return Address{}, fmt.Errorf("pccc: invalid bit %q in %q", suffix, s)
		}
		if bit < 0 || bit > 15 {
			return Address{}, fmt.Errorf("pccc: bit %d out of range [0,15] in %q", bit, s)
		}
		if !supportsBitSuffix(ft) {
			return Address{}, fmt.Errorf("pccc: bit suffix not supported for file type %s in %q",
				ft.Letter(), s)
		}
		addr.BitNum = bit
	}

	return addr, nil
}

// splitSuffix splits up into its main part and a single trailing suffix
// (delimited by '.' or '/'). It returns the leading separator byte (0 if
// no suffix is present), and rejects strings with more than one suffix
// delimiter.
func splitSuffix(up string) (main, suffix string, sep byte) {
	dot := strings.IndexByte(up, '.')
	slash := strings.IndexByte(up, '/')
	switch {
	case dot < 0 && slash < 0:
		return up, "", 0
	case dot >= 0 && slash >= 0:
		// Both present — pick the leftmost; the right will be caught as
		// trailing garbage when we recurse into the suffix.
		idx := dot
		sep = '.'
		if slash < dot {
			idx = slash
			sep = '/'
		}
		main, suffix = up[:idx], up[idx+1:]
		return main, suffix, sep
	case dot >= 0:
		main, suffix = up[:dot], up[dot+1:]
		return main, suffix, '.'
	default:
		main, suffix = up[:slash], up[slash+1:]
		return main, suffix, '/'
	}
}

// splitLeadingLetters splits s into its leading run of ASCII letters and
// everything after. Used to separate the file-type mnemonic from the file
// number (e.g. "N7" -> "N","7"; "ST10" -> "ST","10").
func splitLeadingLetters(s string) (letters, rest string) {
	i := 0
	for i < len(s) && isAlpha(s[i]) {
		i++
	}
	return s[:i], s[i:]
}

func isAlpha(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return true
}

// isAllDigitsOrLeadingSign allows an optional leading '-' so that
// validation can produce an out-of-range error (rather than a parse error)
// for inputs like "N7:-1".
func isAllDigitsOrLeadingSign(s string) bool {
	if s == "" {
		return false
	}
	if s[0] == '-' || s[0] == '+' {
		return len(s) > 1 && isAllDigits(s[1:])
	}
	return isAllDigits(s)
}

// implicitFileNumber returns the standard SLC file number for file types
// whose number is fixed by convention (S=2, I=1, O=0). For other types the
// second return value is false and the caller must supply a file number.
func implicitFileNumber(ft FileType) (int, bool) {
	switch ft {
	case FileTypeOutput:
		return 0, true
	case FileTypeInput:
		return 1, true
	case FileTypeStatus:
		return 2, true
	default:
		return 0, false
	}
}

// supportsBitSuffix reports whether bit addressing (/N) is meaningful for
// the given file type. SLC bit addressing applies to word-sized integer
// and bit files; it is not defined for float, string, or ASCII files.
func supportsBitSuffix(ft FileType) bool {
	switch ft {
	case FileTypeInteger, FileTypeBit, FileTypeStatus, FileTypeInput, FileTypeOutput, FileTypeBCD:
		return true
	default:
		return false
	}
}

// namedField describes a timer/counter/control sub-field. SubElement
// selects the word inside the multi-word element; BitNum is -1 unless the
// field names a specific bit inside that word.
type namedField struct {
	SubElement int
	BitNum     int
}

// timerFields covers SLC 500 timer element layout (3 words):
//
//	word 0 (control): EN bit 15, TT bit 14, DN bit 13
//	word 1: PRE
//	word 2: ACC
var timerFields = map[string]namedField{
	"PRE": {1, -1},
	"ACC": {2, -1},
	"EN":  {0, 15},
	"TT":  {0, 14},
	"DN":  {0, 13},
}

// counterFields covers SLC 500 counter element layout (3 words):
//
//	word 0 (control): CU 15, CD 14, DN 13, OV 12, UN 11, UA 10
//	word 1: PRE
//	word 2: ACC
var counterFields = map[string]namedField{
	"PRE": {1, -1},
	"ACC": {2, -1},
	"CU":  {0, 15},
	"CD":  {0, 14},
	"DN":  {0, 13},
	"OV":  {0, 12},
	"UN":  {0, 11},
	"UA":  {0, 10},
}

// controlFields covers SLC 500 generic control element layout (3 words):
//
//	word 0 (control): EN 15, EU 14, DN 13, EM 12, ER 11, UL 10, IN 9, FD 8
//	word 1: LEN (length)
//	word 2: POS (position)
var controlFields = map[string]namedField{
	"LEN": {1, -1},
	"POS": {2, -1},
	"EN":  {0, 15},
	"EU":  {0, 14},
	"DN":  {0, 13},
	"EM":  {0, 12},
	"ER":  {0, 11},
	"UL":  {0, 10},
	"IN":  {0, 9},
	"FD":  {0, 8},
}

func lookupNamedField(ft FileType, name string) (sub, bit int, ok bool) {
	table := tableFor(ft)
	if table == nil {
		return 0, 0, false
	}
	f, found := table[name]
	if !found {
		return 0, 0, false
	}
	return f.SubElement, f.BitNum, true
}

func tableFor(ft FileType) map[string]namedField {
	switch ft {
	case FileTypeTimer:
		return timerFields
	case FileTypeCounter:
		return counterFields
	case FileTypeControl:
		return controlFields
	default:
		return nil
	}
}

// fieldNameFor returns the canonical field mnemonic (e.g. "ACC", "EN")
// for a (file type, sub-element, bit) tuple, when one exists.
func fieldNameFor(ft FileType, sub, bit int) (string, bool) {
	table := tableFor(ft)
	if table == nil {
		return "", false
	}
	for name, f := range table {
		if f.SubElement == sub && f.BitNum == bit {
			return name, true
		}
	}
	return "", false
}
