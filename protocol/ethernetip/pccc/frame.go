package pccc

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// FileType is a PCCC data-table file type code, as defined in the
// Allen-Bradley DF1 Protocol and Command Set Reference Manual (publication
// 1770-6.5.16). The codes here cover the file types accessible via SLC
// typed read/write (FNC 0xA2 / 0xAA).
type FileType byte

// Standard SLC / MicroLogix file type codes.
const (
	FileTypeStatus  FileType = 0x84 // S — status
	FileTypeBit     FileType = 0x85 // B — bit
	FileTypeTimer   FileType = 0x86 // T — timer (control/PRE/ACC)
	FileTypeCounter FileType = 0x87 // C — counter (control/PRE/ACC)
	FileTypeControl FileType = 0x88 // R — generic control
	FileTypeInteger FileType = 0x89 // N — 16-bit integer
	FileTypeFloat   FileType = 0x8A // F — 32-bit IEEE float
	FileTypeOutput  FileType = 0x8B // O — output image
	FileTypeInput   FileType = 0x8C // I — input image
	FileTypeString  FileType = 0x8D // ST — string
	FileTypeASCII   FileType = 0x8E // A — ASCII
	FileTypeBCD     FileType = 0x8F // D — BCD
)

// Letter returns the single- or two-letter SLC mnemonic for the file type
// (e.g. "N", "F", "ST"). It returns the empty string for unknown codes.
func (ft FileType) Letter() string {
	for _, e := range fileTypeTable {
		if e.ft == ft {
			return e.letter
		}
	}
	return ""
}

// String returns a human-readable name including the mnemonic and hex code.
func (ft FileType) String() string {
	if l := ft.Letter(); l != "" {
		return fmt.Sprintf("%s(0x%02X)", l, byte(ft))
	}
	return fmt.Sprintf("FileType(0x%02X)", byte(ft))
}

// FileTypeFromLetter returns the FileType for an SLC mnemonic like "N", "F",
// "ST". Matching is case-insensitive. The second return value reports
// whether the letter is recognised.
func FileTypeFromLetter(letter string) (FileType, bool) {
	up := strings.ToUpper(letter)
	for _, e := range fileTypeTable {
		if e.letter == up {
			return e.ft, true
		}
	}
	return 0, false
}

type fileTypeEntry struct {
	ft     FileType
	letter string
}

// Ordered so that two-letter mnemonics like "ST" are checked alongside the
// single-letter set without ambiguity.
var fileTypeTable = []fileTypeEntry{
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

// PCCC command and function constants. Only the SLC typed-logical commands
// are defined here; PLC-5 word-range commands are out of scope.
const (
	// CmdProtectedTypedLogical is the PCCC CMD byte used by FNC 0xA2 (read)
	// and FNC 0xAA (write) — "Protected Typed Logical Read/Write with 3
	// Address Fields".
	CmdProtectedTypedLogical byte = 0x0F

	// FuncProtectedTypedLogicalRead is the FNC code for typed-logical read.
	FuncProtectedTypedLogicalRead byte = 0xA2

	// FuncProtectedTypedLogicalWrite is the FNC code for typed-logical write.
	FuncProtectedTypedLogicalWrite byte = 0xAA

	// ReplyBit is OR-ed into the CMD byte of a PCCC reply to distinguish it
	// from a request.
	ReplyBit byte = 0x40

	// StatusUseExtSTS indicates that the EXT STS byte carries the real
	// error code. When STS == 0xF0 the byte immediately following the TNS
	// is the EXT STS code.
	StatusUseExtSTS byte = 0xF0
)

// Reply is a decoded PCCC reply packet.
type Reply struct {
	// Command is the request CMD with the reply bit (0x40) stripped.
	Command byte
	// STS is the PCCC status code (0 = success).
	STS byte
	// TNS is the transaction number echoed from the request.
	TNS uint16
	// ExtSTS is the extended status byte. Non-zero only when STS == 0xF0.
	ExtSTS byte
	// Data is the application payload following the header (may be empty,
	// e.g. for a write reply).
	Data []byte
}

// Error represents a PCCC protocol error: a non-zero STS, optionally with
// an EXT STS code. Like cip.Error and modbus.ModbusError, callers should
// treat these as protocol-level failures and not retry them.
type Error struct {
	STS    byte
	ExtSTS byte
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.STS == StatusUseExtSTS {
		return fmt.Sprintf("pccc: STS=0x%02X EXT_STS=0x%02X (%s)",
			e.STS, e.ExtSTS, StatusMessage(e.STS, e.ExtSTS))
	}
	return fmt.Sprintf("pccc: STS=0x%02X (%s)", e.STS, StatusMessage(e.STS, e.ExtSTS))
}

// IsPCCCError reports whether err is a [*Error] returned by this package.
func IsPCCCError(err error) bool {
	_, ok := err.(*Error)
	return ok
}

// EncodeTypedRead encodes a PROTECTED TYPED LOGICAL READ (FNC 0xA2) command.
// The returned bytes are the PCCC message (CMD..end) ready to be wrapped in
// an Execute_PCCC CIP request.
//
// byteSize is the number of data-table bytes to read (1..255). For an INT
// read use 2; for a REAL use 4; for n INTs use n*2.
func EncodeTypedRead(tns uint16, byteSize, fileNum int, ft FileType, elem, subElem int) ([]byte, error) {
	if err := validateAddress(fileNum, elem, subElem, ft); err != nil {
		return nil, err
	}
	if byteSize < 1 || byteSize > 255 {
		return nil, fmt.Errorf("pccc: byte size %d out of range [1,255]", byteSize)
	}
	buf := make([]byte, 10)
	buf[0] = CmdProtectedTypedLogical
	buf[1] = 0x00 // STS — always 0 in requests
	binary.LittleEndian.PutUint16(buf[2:4], tns)
	buf[4] = FuncProtectedTypedLogicalRead
	buf[5] = byte(byteSize)
	buf[6] = byte(fileNum)
	buf[7] = byte(ft)
	buf[8] = byte(elem)
	buf[9] = byte(subElem)
	return buf, nil
}

// EncodeTypedWrite encodes a PROTECTED TYPED LOGICAL WRITE (FNC 0xAA)
// command. data is the raw payload to be written into the file; the caller
// is responsible for ensuring the length matches the addressed element(s).
func EncodeTypedWrite(tns uint16, fileNum int, ft FileType, elem, subElem int, data []byte) ([]byte, error) {
	if err := validateAddress(fileNum, elem, subElem, ft); err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("pccc: write data is empty")
	}
	if len(data) > 240 {
		// Conservative cap; a real PCCC frame fits in ~240 bytes after the
		// CIP/EIP overhead. Real callers should chunk before reaching this
		// limit; the framing layer just rejects obviously oversized writes.
		return nil, fmt.Errorf("pccc: write data too large (%d bytes)", len(data))
	}
	buf := make([]byte, 9+len(data))
	buf[0] = CmdProtectedTypedLogical
	buf[1] = 0x00
	binary.LittleEndian.PutUint16(buf[2:4], tns)
	buf[4] = FuncProtectedTypedLogicalWrite
	buf[5] = byte(fileNum)
	buf[6] = byte(ft)
	buf[7] = byte(elem)
	buf[8] = byte(subElem)
	copy(buf[9:], data)
	return buf, nil
}

func validateAddress(fileNum, elem, subElem int, ft FileType) error {
	if ft.Letter() == "" {
		return fmt.Errorf("pccc: unknown file type 0x%02X", byte(ft))
	}
	if fileNum < 0 || fileNum > 255 {
		return fmt.Errorf("pccc: file number %d out of range [0,255]", fileNum)
	}
	if elem < 0 || elem > 255 {
		return fmt.Errorf("pccc: element %d out of range [0,255]", elem)
	}
	if subElem < 0 || subElem > 255 {
		return fmt.Errorf("pccc: sub-element %d out of range [0,255]", subElem)
	}
	return nil
}

// DecodeReply parses a PCCC reply frame. On success, the returned [Reply]
// has STS == 0 and Data holds the application payload. On a non-zero STS,
// the error is a [*Error] (test with [IsPCCCError] or [errors.As]).
func DecodeReply(raw []byte) (Reply, error) {
	if len(raw) < 4 {
		return Reply{}, fmt.Errorf("pccc: reply too short (%d bytes, need >=4)", len(raw))
	}
	if raw[0]&ReplyBit == 0 {
		return Reply{}, fmt.Errorf("pccc: CMD 0x%02X has no reply bit set", raw[0])
	}
	r := Reply{
		Command: raw[0] &^ ReplyBit,
		STS:     raw[1],
		TNS:     binary.LittleEndian.Uint16(raw[2:4]),
	}
	rest := raw[4:]
	if r.STS == StatusUseExtSTS {
		if len(rest) < 1 {
			return Reply{}, fmt.Errorf("pccc: STS=0xF0 but reply missing EXT STS byte")
		}
		r.ExtSTS = rest[0]
		rest = rest[1:]
	}
	if r.STS != 0 {
		return r, &Error{STS: r.STS, ExtSTS: r.ExtSTS}
	}
	if len(rest) > 0 {
		r.Data = make([]byte, len(rest))
		copy(r.Data, rest)
	}
	return r, nil
}

// StatusMessage returns a human-readable description of a PCCC status. For
// STS != 0xF0 the message is determined by sts alone; for STS == 0xF0 the
// extSTS byte is decoded. Unknown codes fall back to a hex representation.
func StatusMessage(sts, extSTS byte) string {
	if sts == 0 {
		return "success"
	}
	if sts == StatusUseExtSTS {
		if msg, ok := extStatusMessages[extSTS]; ok {
			return msg
		}
		return fmt.Sprintf("extended status 0x%02X", extSTS)
	}
	if msg, ok := localStatusMessages[sts]; ok {
		return msg
	}
	return fmt.Sprintf("status 0x%02X", sts)
}

// Local STS messages — Allen-Bradley DF1 Reference Manual, Appendix B.
// The local (low-nibble) and remote (high-nibble) sets are merged into a
// single byte-keyed table; values map to the encoded byte as transmitted.
var localStatusMessages = map[byte]string{
	// Local errors (low nibble)
	0x01: "local: destination node out of buffer space",
	0x02: "local: cannot guarantee delivery (link layer)",
	0x03: "local: duplicate token holder detected",
	0x04: "local: local port disconnected",
	0x05: "local: application timed out waiting for response",
	0x06: "local: duplicate node detected",
	0x07: "local: station is offline",
	0x08: "local: hardware fault",

	// Remote errors (high nibble); textbook DF1 codes.
	0x10: "remote: illegal command or format",
	0x20: "remote: host has a problem and will not communicate",
	0x30: "remote: remote node host is missing, disconnected, or shut down",
	0x40: "remote: host could not complete function due to hardware fault",
	0x50: "remote: addressing problem or memory protect rungs",
	0x60: "remote: function not allowed due to command-protection selection",
	0x70: "remote: processor is in program mode",
	0x80: "remote: compatibility-mode file missing or communication-zone problem",
	0x90: "remote: remote node cannot buffer command",
	0xA0: "remote: wait ACK (1775-KA buffer full)",
	0xB0: "remote: remote node problem due to download",
	0xC0: "remote: wait ACK (1775-KA buffer full)",
}

// EXT STS messages — Allen-Bradley DF1 Reference Manual, Appendix B,
// table for STS = 0xF0.
var extStatusMessages = map[byte]string{
	0x01: "a field has an illegal value",
	0x02: "fewer levels specified in address than minimum",
	0x03: "more levels specified in address than system supports",
	0x04: "symbol not found",
	0x05: "symbol is of improper format",
	0x06: "address does not point to something usable",
	0x07: "file is wrong size",
	0x08: "cannot complete request; situation changed since command started",
	0x09: "data or file is too large",
	0x0A: "transaction size plus word address is too large",
	0x0B: "access denied, improper privilege",
	0x0C: "condition cannot be generated; resource is not available",
	0x0D: "condition already exists; resource is already available",
	0x0E: "command cannot be executed",
	0x0F: "histogram overflow",
	0x10: "no access",
	0x11: "illegal data type",
	0x12: "invalid parameter or invalid data",
	0x13: "address reference exists to deleted area",
	0x14: "command execution failure for unknown reason",
	0x15: "data conversion error",
	0x16: "scanner not able to communicate with 1771 rack adapter",
	0x17: "type mismatch",
	0x18: "1771 module response was not valid",
	0x19: "duplicated label",
	0x1A: "file is open; another node owns it",
	0x1B: "another node is the program owner",
	0x1E: "data table element protection violation",
	0x1F: "temporary internal problem",
}
