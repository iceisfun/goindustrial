package pccc

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// ===========================================================================
// File type code values (Allen-Bradley DF1 Reference Manual, 1770-6.5.16)
// ===========================================================================

func TestFileTypeCodes(t *testing.T) {
	tests := []struct {
		name string
		ft   FileType
		code byte
	}{
		{"Status", FileTypeStatus, 0x84},
		{"Bit", FileTypeBit, 0x85},
		{"Timer", FileTypeTimer, 0x86},
		{"Counter", FileTypeCounter, 0x87},
		{"Control", FileTypeControl, 0x88},
		{"Integer", FileTypeInteger, 0x89},
		{"Float", FileTypeFloat, 0x8A},
		{"Output", FileTypeOutput, 0x8B},
		{"Input", FileTypeInput, 0x8C},
		{"String", FileTypeString, 0x8D},
		{"ASCII", FileTypeASCII, 0x8E},
		{"BCD", FileTypeBCD, 0x8F},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if byte(tc.ft) != tc.code {
				t.Fatalf("%s: got 0x%02X want 0x%02X", tc.name, byte(tc.ft), tc.code)
			}
		})
	}
}

func TestFileTypeLetter(t *testing.T) {
	tests := []struct {
		ft     FileType
		letter string
	}{
		{FileTypeStatus, "S"},
		{FileTypeBit, "B"},
		{FileTypeTimer, "T"},
		{FileTypeCounter, "C"},
		{FileTypeControl, "R"},
		{FileTypeInteger, "N"},
		{FileTypeFloat, "F"},
		{FileTypeOutput, "O"},
		{FileTypeInput, "I"},
		{FileTypeString, "ST"},
		{FileTypeASCII, "A"},
		{FileTypeBCD, "D"},
	}
	for _, tc := range tests {
		t.Run(tc.letter, func(t *testing.T) {
			if got := tc.ft.Letter(); got != tc.letter {
				t.Fatalf("letter: got %q want %q", got, tc.letter)
			}
		})
	}
}

func TestFileTypeFromLetter(t *testing.T) {
	tests := []struct {
		letter string
		ft     FileType
		ok     bool
	}{
		{"S", FileTypeStatus, true},
		{"B", FileTypeBit, true},
		{"N", FileTypeInteger, true},
		{"F", FileTypeFloat, true},
		{"T", FileTypeTimer, true},
		{"C", FileTypeCounter, true},
		{"R", FileTypeControl, true},
		{"O", FileTypeOutput, true},
		{"I", FileTypeInput, true},
		{"ST", FileTypeString, true},
		{"A", FileTypeASCII, true},
		{"D", FileTypeBCD, true},
		// case sensitivity: SLC mnemonics are upper-case; lower-case allowed
		{"n", FileTypeInteger, true},
		{"st", FileTypeString, true},
		{"", 0, false},
		{"X", 0, false},
		{"NN", 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.letter, func(t *testing.T) {
			got, ok := FileTypeFromLetter(tc.letter)
			if ok != tc.ok {
				t.Fatalf("ok: got %v want %v", ok, tc.ok)
			}
			if ok && got != tc.ft {
				t.Fatalf("file type: got 0x%02X want 0x%02X", byte(got), byte(tc.ft))
			}
		})
	}
}

// ===========================================================================
// Command / FNC / reply bit constants
// ===========================================================================

func TestPCCCConstants(t *testing.T) {
	if CmdProtectedTypedLogical != 0x0F {
		t.Errorf("CmdProtectedTypedLogical: got 0x%02X want 0x0F", CmdProtectedTypedLogical)
	}
	if FuncProtectedTypedLogicalRead != 0xA2 {
		t.Errorf("FuncProtectedTypedLogicalRead: got 0x%02X want 0xA2", FuncProtectedTypedLogicalRead)
	}
	if FuncProtectedTypedLogicalWrite != 0xAA {
		t.Errorf("FuncProtectedTypedLogicalWrite: got 0x%02X want 0xAA", FuncProtectedTypedLogicalWrite)
	}
	if ReplyBit != 0x40 {
		t.Errorf("ReplyBit: got 0x%02X want 0x40", ReplyBit)
	}
	if StatusUseExtSTS != 0xF0 {
		t.Errorf("StatusUseExtSTS: got 0x%02X want 0xF0", StatusUseExtSTS)
	}
}

// ===========================================================================
// EncodeTypedRead
//
// PCCC FNC 0xA2 (PROTECTED TYPED LOGICAL READ with 3 address fields):
//   CMD(0x0F) STS(0x00) TNS(LE u16) FNC(0xA2) ByteSize(u8)
//   FileNum(u8) FileType(u8) Element(u8) SubElement(u8)
// ===========================================================================

func TestEncodeTypedRead(t *testing.T) {
	// Reading 2 bytes (1 INT) of N7:0:
	//   CMD=0x0F STS=0x00 TNS=0x3412 FNC=0xA2 size=2 file=7 type=N(0x89) elem=0 sub=0
	got, err := EncodeTypedRead(0x1234, 2, 7, FileTypeInteger, 0, 0)
	if err != nil {
		t.Fatalf("EncodeTypedRead: %v", err)
	}
	want := []byte{0x0F, 0x00, 0x34, 0x12, 0xA2, 0x02, 0x07, 0x89, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("encoded bytes mismatch\n got: % X\nwant: % X", got, want)
	}
}

func TestEncodeTypedReadTimerACC(t *testing.T) {
	// Reading T4:5.ACC — element 5, sub-element 2 (ACC), 2 bytes.
	got, err := EncodeTypedRead(0x0001, 2, 4, FileTypeTimer, 5, 2)
	if err != nil {
		t.Fatalf("EncodeTypedRead: %v", err)
	}
	want := []byte{0x0F, 0x00, 0x01, 0x00, 0xA2, 0x02, 0x04, 0x86, 0x05, 0x02}
	if !bytes.Equal(got, want) {
		t.Fatalf("encoded bytes mismatch\n got: % X\nwant: % X", got, want)
	}
}

func TestEncodeTypedReadValidates(t *testing.T) {
	tests := []struct {
		name                                 string
		byteSize, fileNum, elem, subElem int
		ft                               FileType
	}{
		{"zero byte size", 0, 7, 0, 0, FileTypeInteger},
		{"byte size > 255", 300, 7, 0, 0, FileTypeInteger},
		{"file num > 255", 1, 999, 0, 0, FileTypeInteger},
		{"file num negative", 1, -1, 0, 0, FileTypeInteger},
		{"elem > 255", 1, 7, 999, 0, FileTypeInteger},
		{"sub-elem > 255", 1, 7, 0, 999, FileTypeInteger},
		{"unknown file type", 1, 7, 0, 0, FileType(0x00)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := EncodeTypedRead(1, tc.byteSize, tc.fileNum, tc.ft, tc.elem, tc.subElem); err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}

// ===========================================================================
// EncodeTypedWrite
//
// PCCC FNC 0xAA (PROTECTED TYPED LOGICAL WRITE with 3 address fields):
//   CMD(0x0F) STS(0x00) TNS(LE u16) FNC(0xAA)
//   FileNum(u8) FileType(u8) Element(u8) SubElement(u8) Data...
// Note: typed write has no explicit byte-size field — the size is implicit
// in the data length (caller's responsibility).
// ===========================================================================

func TestEncodeTypedWrite(t *testing.T) {
	// Write 0x002A to N7:0
	data := []byte{0x2A, 0x00}
	got, err := EncodeTypedWrite(0x0001, 7, FileTypeInteger, 0, 0, data)
	if err != nil {
		t.Fatalf("EncodeTypedWrite: %v", err)
	}
	want := []byte{0x0F, 0x00, 0x01, 0x00, 0xAA, 0x07, 0x89, 0x00, 0x00, 0x2A, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("encoded bytes mismatch\n got: % X\nwant: % X", got, want)
	}
}

func TestEncodeTypedWriteValidates(t *testing.T) {
	tests := []struct {
		name                          string
		fileNum, elem, subElem int
		ft                     FileType
		data                   []byte
	}{
		{"nil data", 7, 0, 0, FileTypeInteger, nil},
		{"empty data", 7, 0, 0, FileTypeInteger, []byte{}},
		{"file num > 255", 999, 0, 0, FileTypeInteger, []byte{0, 0}},
		{"elem > 255", 7, 999, 0, FileTypeInteger, []byte{0, 0}},
		{"sub-elem > 255", 7, 0, 999, FileTypeInteger, []byte{0, 0}},
		{"unknown file type", 7, 0, 0, FileType(0), []byte{0, 0}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := EncodeTypedWrite(1, tc.fileNum, tc.ft, tc.elem, tc.subElem, tc.data); err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}

// ===========================================================================
// DecodeReply
//
// Reply format:
//   CMD(req|0x40) STS(u8) TNS(LE u16) [EXT_STS(u8) if STS==0xF0] Data...
// ===========================================================================

func TestDecodeReplySuccess(t *testing.T) {
	// Reply to a typed read: CMD=0x4F, STS=0, TNS=0x1234, data=0x002A
	raw := []byte{0x4F, 0x00, 0x34, 0x12, 0x2A, 0x00}
	r, err := DecodeReply(raw)
	if err != nil {
		t.Fatalf("DecodeReply: %v", err)
	}
	if r.Command != CmdProtectedTypedLogical {
		t.Errorf("Command: got 0x%02X want 0x0F (reply bit stripped)", r.Command)
	}
	if r.TNS != 0x1234 {
		t.Errorf("TNS: got 0x%04X want 0x1234", r.TNS)
	}
	if r.STS != 0 {
		t.Errorf("STS: got 0x%02X want 0", r.STS)
	}
	if r.ExtSTS != 0 {
		t.Errorf("ExtSTS: got 0x%02X want 0", r.ExtSTS)
	}
	if !bytes.Equal(r.Data, []byte{0x2A, 0x00}) {
		t.Errorf("Data: got % X want 2A 00", r.Data)
	}
}

func TestDecodeReplyEmptyData(t *testing.T) {
	// Reply to a typed write: no data payload on success.
	raw := []byte{0x4F, 0x00, 0x01, 0x00}
	r, err := DecodeReply(raw)
	if err != nil {
		t.Fatalf("DecodeReply: %v", err)
	}
	if len(r.Data) != 0 {
		t.Errorf("Data: got % X want empty", r.Data)
	}
	if r.STS != 0 {
		t.Errorf("STS: got 0x%02X want 0", r.STS)
	}
}

func TestDecodeReplyLocalSTSError(t *testing.T) {
	// STS = 0x10 (illegal command/format). No EXT STS byte.
	raw := []byte{0x4F, 0x10, 0x34, 0x12}
	_, err := DecodeReply(raw)
	if err == nil {
		t.Fatalf("expected error for STS=0x10, got nil")
	}
	if !IsPCCCError(err) {
		t.Fatalf("expected pccc.Error, got %T: %v", err, err)
	}
	var pe *Error
	if !errors.As(err, &pe) {
		t.Fatalf("errors.As to *pccc.Error failed: %v", err)
	}
	if pe.STS != 0x10 {
		t.Errorf("Error.STS: got 0x%02X want 0x10", pe.STS)
	}
	if pe.ExtSTS != 0 {
		t.Errorf("Error.ExtSTS: got 0x%02X want 0", pe.ExtSTS)
	}
	if !strings.Contains(pe.Error(), "0x10") {
		t.Errorf("error string %q should mention STS code", pe.Error())
	}
}

func TestDecodeReplyExtSTSError(t *testing.T) {
	// STS = 0xF0 with EXT STS = 0x04 (symbol not found)
	raw := []byte{0x4F, 0xF0, 0x34, 0x12, 0x04}
	_, err := DecodeReply(raw)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	var pe *Error
	if !errors.As(err, &pe) {
		t.Fatalf("errors.As to *pccc.Error failed: %v", err)
	}
	if pe.STS != 0xF0 || pe.ExtSTS != 0x04 {
		t.Errorf("STS/ExtSTS: got 0x%02X/0x%02X want 0xF0/0x04", pe.STS, pe.ExtSTS)
	}
	if !strings.Contains(pe.Error(), "0x04") {
		t.Errorf("error string %q should mention EXT STS code", pe.Error())
	}
}

func TestDecodeReplyTruncated(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
	}{
		{"empty", nil},
		{"only cmd", []byte{0x4F}},
		{"only cmd+sts", []byte{0x4F, 0x00}},
		{"missing tns high", []byte{0x4F, 0x00, 0x34}},
		{"ext sts marker but no ext byte", []byte{0x4F, 0xF0, 0x34, 0x12}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := DecodeReply(tc.raw); err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}

func TestDecodeReplyRejectsRequest(t *testing.T) {
	// CMD without reply bit set should be rejected — caller passed a
	// request rather than a reply.
	raw := []byte{0x0F, 0x00, 0x34, 0x12, 0xA2, 0x02, 0x07, 0x89, 0x00, 0x00}
	if _, err := DecodeReply(raw); err == nil {
		t.Fatalf("expected error for request-shaped frame, got nil")
	}
}

// ===========================================================================
// Status-code messages
// ===========================================================================

func TestStatusMessage(t *testing.T) {
	// A couple of well-known codes — exact wording may vary, just confirm
	// non-empty and code-specific.
	tests := []struct {
		sts, ext byte
		mustHave string
	}{
		{0x10, 0x00, "illegal"},
		{0x70, 0x00, "program mode"},
		{0xF0, 0x04, "symbol not found"},
	}
	for _, tc := range tests {
		msg := StatusMessage(tc.sts, tc.ext)
		if msg == "" {
			t.Errorf("StatusMessage(0x%02X,0x%02X) empty", tc.sts, tc.ext)
		}
		if !strings.Contains(strings.ToLower(msg), tc.mustHave) {
			t.Errorf("StatusMessage(0x%02X,0x%02X)=%q should contain %q", tc.sts, tc.ext, msg, tc.mustHave)
		}
	}
}

func TestStatusMessageUnknown(t *testing.T) {
	// Unknown code falls back to hex.
	msg := StatusMessage(0x7B, 0x00)
	if !strings.Contains(msg, "0x7B") {
		t.Errorf("StatusMessage(0x7B) %q should include the hex code", msg)
	}
}

// ===========================================================================
// Round-trip: encode request, simulate stub reply, decode.
// ===========================================================================

func TestEncodeDecodeRoundTrip(t *testing.T) {
	req, err := EncodeTypedRead(0xBEEF, 2, 7, FileTypeInteger, 0, 0)
	if err != nil {
		t.Fatalf("EncodeTypedRead: %v", err)
	}
	// A stub server would echo CMD|0x40 and TNS, then append data.
	tnsLo, tnsHi := req[2], req[3]
	reply := []byte{req[0] | ReplyBit, 0x00, tnsLo, tnsHi, 0x2A, 0x00}
	r, err := DecodeReply(reply)
	if err != nil {
		t.Fatalf("DecodeReply: %v", err)
	}
	if r.TNS != 0xBEEF {
		t.Fatalf("round-trip TNS: got 0x%04X want 0xBEEF", r.TNS)
	}
}
