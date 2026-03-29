package hexdump

import (
	"fmt"
	"io"
	"strings"
	"sync"
)

// Direction indicates whether data was read or written.
type Direction int

const (
	// DirRead marks data received from the remote side.
	DirRead Direction = iota
	// DirWrite marks data sent to the remote side.
	DirWrite
)

// Dumper writes hex dump traces of data passing through wrapped readers and
// writers. All output is serialized so concurrent reads and writes produce
// clean, non-interleaved dump blocks. A Dumper is safe for concurrent use.
type Dumper struct {
	out io.Writer
	mu  sync.Mutex
}

// NewDumper creates a Dumper that writes hex dump output to out.
func NewDumper(out io.Writer) *Dumper {
	return &Dumper{out: out}
}

// WrapReader returns an [io.Reader] that, for every Read call, writes a hex
// dump of the data read (with <<< READ direction) to the Dumper's output.
func (d *Dumper) WrapReader(r io.Reader) io.Reader {
	return &dumpReader{r: r, d: d}
}

// WrapWriter returns an [io.Writer] that, for every Write call, writes a hex
// dump of the data written (with >>> WRITE direction) to the Dumper's output.
func (d *Dumper) WrapWriter(w io.Writer) io.Writer {
	return &dumpWriter{w: w, d: d}
}

// Dump formats data in traditional hex dump style and writes it to the
// Dumper's output. The direction marker and byte count are printed first,
// followed by offset/hex/ASCII lines. Short final lines are space-padded so
// the ASCII column always aligns.
func (d *Dumper) Dump(data []byte, dir Direction) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var b strings.Builder
	formatDump(&b, data, dir)
	fmt.Fprint(d.out, b.String())
}

// formatDump writes a complete hex dump block to b.
func formatDump(b *strings.Builder, data []byte, dir Direction) {
	switch dir {
	case DirRead:
		fmt.Fprintf(b, "<<< READ %d bytes\n", len(data))
	case DirWrite:
		fmt.Fprintf(b, ">>> WRITE %d bytes\n", len(data))
	}

	if len(data) == 0 {
		return
	}

	for i := 0; i < len(data); i += 16 {
		// Offset column.
		fmt.Fprintf(b, "%08x  ", i)

		// Hex columns: 16 bytes with extra space between byte 7 and 8.
		for j := range 16 {
			if j == 8 {
				b.WriteByte(' ')
			}
			if i+j < len(data) {
				fmt.Fprintf(b, "%02x ", data[i+j])
			} else {
				b.WriteString("   ")
			}
		}

		// ASCII column.
		b.WriteByte('|')
		for j := range 16 {
			if i+j < len(data) {
				c := data[i+j]
				if c >= 0x20 && c <= 0x7e {
					b.WriteByte(c)
				} else {
					b.WriteByte('.')
				}
			} else {
				b.WriteByte(' ')
			}
		}
		b.WriteByte('|')
		b.WriteByte('\n')
	}
}

// dumpReader wraps an io.Reader and dumps all data read.
type dumpReader struct {
	r io.Reader
	d *Dumper
}

func (r *dumpReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if n > 0 {
		r.d.Dump(p[:n], DirRead)
	}
	return n, err
}

// dumpWriter wraps an io.Writer and dumps all data written.
type dumpWriter struct {
	w io.Writer
	d *Dumper
}

func (w *dumpWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	if n > 0 {
		w.d.Dump(p[:n], DirWrite)
	}
	return n, err
}
