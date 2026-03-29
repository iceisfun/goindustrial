package hexdump

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestDumpEmpty(t *testing.T) {
	var buf bytes.Buffer
	d := NewDumper(&buf)
	d.Dump(nil, DirWrite)

	got := buf.String()
	if !strings.Contains(got, ">>> WRITE 0 bytes") {
		t.Errorf("expected direction header for empty data, got:\n%s", got)
	}
	// Should only have the header line, no hex rows.
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line for empty dump, got %d:\n%s", len(lines), got)
	}
}

func TestDumpSingleLine(t *testing.T) {
	var buf bytes.Buffer
	d := NewDumper(&buf)

	data := make([]byte, 16)
	for i := range data {
		data[i] = byte(i)
	}
	d.Dump(data, DirWrite)

	got := buf.String()

	// Verify direction header.
	if !strings.Contains(got, ">>> WRITE 16 bytes") {
		t.Errorf("missing direction header:\n%s", got)
	}

	// Verify hex row contains all 16 bytes.
	if !strings.Contains(got, "00000000") {
		t.Errorf("missing offset:\n%s", got)
	}
	if !strings.Contains(got, "00 01 02 03 04 05 06 07  08 09 0a 0b 0c 0d 0e 0f") {
		t.Errorf("hex content mismatch:\n%s", got)
	}

	// Verify ASCII column is exactly 16 chars wide with pipes.
	if !strings.Contains(got, "|................|") {
		t.Errorf("ASCII column mismatch:\n%s", got)
	}
}

func TestDumpMultiLine(t *testing.T) {
	var buf bytes.Buffer
	d := NewDumper(&buf)

	data := make([]byte, 32)
	for i := range data {
		data[i] = byte(i + 0x40) // @, A, B, ...
	}
	d.Dump(data, DirRead)

	got := buf.String()

	if !strings.Contains(got, "<<< READ 32 bytes") {
		t.Errorf("missing direction header:\n%s", got)
	}
	if !strings.Contains(got, "00000000") {
		t.Errorf("missing first offset:\n%s", got)
	}
	if !strings.Contains(got, "00000010") {
		t.Errorf("missing second offset:\n%s", got)
	}

	// Verify ASCII for printable range.
	if !strings.Contains(got, "|@ABCDEFGHIJKLMNO|") {
		t.Errorf("first ASCII row mismatch:\n%s", got)
	}
	if !strings.Contains(got, "|PQRSTUVWXYZ[\\]^_|") {
		t.Errorf("second ASCII row mismatch:\n%s", got)
	}
}

func TestDumpShortLastLine(t *testing.T) {
	var buf bytes.Buffer
	d := NewDumper(&buf)

	// 20 bytes: first row full (16), second row partial (4).
	data := make([]byte, 20)
	for i := range data {
		data[i] = byte(i + 0x41) // A, B, C, ...
	}
	d.Dump(data, DirWrite)

	got := buf.String()
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")

	// header + 2 hex lines = 3.
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d:\n%s", len(lines), got)
	}

	firstHexLine := lines[1]
	secondHexLine := lines[2]

	// Both hex lines should have the same total length because the short
	// line is space-padded.
	if len(firstHexLine) != len(secondHexLine) {
		t.Errorf("lines have different lengths (%d vs %d), short line not padded:\nfull:  %q\nshort: %q",
			len(firstHexLine), len(secondHexLine), firstHexLine, secondHexLine)
	}

	// The ASCII column of the short line should be padded with spaces.
	// 4 data bytes + 12 spaces = 16 chars inside pipes.
	if !strings.Contains(secondHexLine, "|QRST            |") {
		t.Errorf("short ASCII row not padded correctly:\n%s", secondHexLine)
	}
}

func TestDumpNonPrintableASCII(t *testing.T) {
	var buf bytes.Buffer
	d := NewDumper(&buf)

	data := []byte{0x00, 0x01, 0x7f, 0x80, 0xff, 'H', 'i', '!'}
	d.Dump(data, DirRead)

	got := buf.String()

	// Non-printable bytes should be dots; 'H', 'i', '!' should be literal.
	if !strings.Contains(got, "|.....Hi!        |") {
		t.Errorf("non-printable ASCII handling wrong:\n%s", got)
	}
}

func TestDumpDirectionMarkers(t *testing.T) {
	var buf bytes.Buffer
	d := NewDumper(&buf)

	d.Dump([]byte{0xAA}, DirWrite)
	d.Dump([]byte{0xBB}, DirRead)

	got := buf.String()
	if !strings.Contains(got, ">>> WRITE 1 bytes") {
		t.Errorf("missing WRITE marker:\n%s", got)
	}
	if !strings.Contains(got, "<<< READ 1 bytes") {
		t.Errorf("missing READ marker:\n%s", got)
	}
}

func TestDumperWrapReader(t *testing.T) {
	var dumpBuf bytes.Buffer
	d := NewDumper(&dumpBuf)

	src := bytes.NewReader([]byte("Hello, World!"))
	wrapped := d.WrapReader(src)

	var readBuf bytes.Buffer
	if _, err := io.Copy(&readBuf, wrapped); err != nil {
		t.Fatal(err)
	}

	// The data should pass through unchanged.
	if readBuf.String() != "Hello, World!" {
		t.Errorf("read data corrupted: %q", readBuf.String())
	}

	// The dump should contain the data.
	got := dumpBuf.String()
	if !strings.Contains(got, "<<< READ") {
		t.Errorf("dump missing READ direction:\n%s", got)
	}
	if !strings.Contains(got, "Hello, World!") {
		t.Errorf("dump missing ASCII data:\n%s", got)
	}
}

func TestDumperWrapWriter(t *testing.T) {
	var dumpBuf bytes.Buffer
	var dataBuf bytes.Buffer
	d := NewDumper(&dumpBuf)

	wrapped := d.WrapWriter(&dataBuf)

	data := []byte("Test write data")
	if _, err := wrapped.Write(data); err != nil {
		t.Fatal(err)
	}

	// The data should pass through unchanged.
	if dataBuf.String() != "Test write data" {
		t.Errorf("written data corrupted: %q", dataBuf.String())
	}

	// The dump should contain the data.
	got := dumpBuf.String()
	if !strings.Contains(got, ">>> WRITE") {
		t.Errorf("dump missing WRITE direction:\n%s", got)
	}
	if !strings.Contains(got, "Test write data") {
		t.Errorf("dump missing ASCII data:\n%s", got)
	}
}

func TestDumperConcurrency(t *testing.T) {
	var dumpBuf bytes.Buffer
	d := NewDumper(&dumpBuf)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			data := []byte{byte(n), byte(n + 1), byte(n + 2)}
			if n%2 == 0 {
				d.Dump(data, DirWrite)
			} else {
				d.Dump(data, DirRead)
			}
		}(i)
	}
	wg.Wait()

	// Verify no panics and output contains expected markers.
	got := dumpBuf.String()
	writeCount := strings.Count(got, ">>> WRITE")
	readCount := strings.Count(got, "<<< READ")
	if writeCount+readCount != 50 {
		t.Errorf("expected 50 dump blocks, got %d writes + %d reads = %d",
			writeCount, readCount, writeCount+readCount)
	}
}

func TestDumpExactly16Bytes(t *testing.T) {
	// Exactly one full line — no padding needed.
	var buf bytes.Buffer
	d := NewDumper(&buf)

	data := []byte("ABCDEFGHIJKLMNOP") // exactly 16
	d.Dump(data, DirWrite)

	got := buf.String()
	if !strings.Contains(got, "|ABCDEFGHIJKLMNOP|") {
		t.Errorf("full 16-byte ASCII row wrong:\n%s", got)
	}
}

func TestDumpOneByte(t *testing.T) {
	var buf bytes.Buffer
	d := NewDumper(&buf)

	d.Dump([]byte{0x42}, DirRead) // 'B'

	got := buf.String()
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")

	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d:\n%s", len(lines), got)
	}

	hexLine := lines[1]
	// ASCII column should be 'B' + 15 spaces inside pipes.
	if !strings.Contains(hexLine, "|B               |") {
		t.Errorf("single byte ASCII row not padded correctly:\n%s", hexLine)
	}
}

func TestDumpAlignmentConsistency(t *testing.T) {
	// Verify that every hex line in a multi-line dump has the same length.
	var buf bytes.Buffer
	d := NewDumper(&buf)

	data := make([]byte, 50)
	for i := range data {
		data[i] = byte(i)
	}
	d.Dump(data, DirWrite)

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	// Skip the header line.
	hexLines := lines[1:]

	if len(hexLines) == 0 {
		t.Fatal("no hex lines")
	}

	expectedLen := len(hexLines[0])
	for i, line := range hexLines {
		if len(line) != expectedLen {
			t.Errorf("line %d has length %d, expected %d:\n%q", i, len(line), expectedLen, line)
		}
	}
}
