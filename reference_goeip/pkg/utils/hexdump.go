package utils

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// HexDump returns a string representation of the data in a hex dump format.
// It is useful for debugging network packets.
func HexDump(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	return hex.Dump(data)
}

// HexDumpLines returns a slice of strings, each representing a line of the hex dump.
// This is useful if you want to prefix each line with a log level or timestamp.
func HexDumpLines(data []byte) []string {
	dump := HexDump(data)
	lines := strings.Split(dump, "\n")
	// hex.Dump adds a trailing newline, so the last element might be empty
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// ByteToHex converts a single byte to a hex string (e.g., "0A").
func ByteToHex(b byte) string {
	return fmt.Sprintf("%02X", b)
}
