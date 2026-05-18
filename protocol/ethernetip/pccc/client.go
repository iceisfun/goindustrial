package pccc

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sync/atomic"

	"github.com/iceisfun/goindustrial/plc"
)

// Executor is the minimum interface a [Client] needs from its underlying
// transport. The production implementation is *ethernetip.Client, whose
// ExecutePCCC ships the bytes via CIP service 0x4B and strips the requestor
// ID from the reply. Tests can substitute a fake.
type Executor interface {
	ExecutePCCC(ctx context.Context, pcccCmd []byte) ([]byte, error)
}

// Connector is an optional extension of [Executor] that participates in the
// connection lifecycle. When the Executor passed to [NewClient] implements
// it, [Client] satisfies [plc.PLC]; otherwise the lifecycle methods are
// no-ops.
type Connector interface {
	Connect(ctx context.Context) error
	Disconnect(ctx context.Context) error
	IsConnected() bool
}

// Client is the high-level PCCC API. It parses SLC addresses, builds and
// decodes PCCC frames, and exposes both convenience methods
// ([Client.ReadAddress], [Client.WriteAddress], [Client.ReadWords]) and the
// protocol-agnostic [plc.Reader] / [plc.Writer] interface using [File]
// data points.
type Client struct {
	exec Executor
	tns  atomic.Uint32 // wrapped to 16 bits on use
}

// NewClient creates a high-level PCCC client backed by exec.
func NewClient(exec Executor, opts ...Option) *Client {
	c := &Client{exec: exec}
	for _, o := range opts {
		o(c)
	}
	// Seed the TNS counter with a small non-zero value so the first
	// transaction is easy to spot on a wire trace.
	c.tns.Store(1)
	return c
}

// Option configures a [Client] created by [NewClient].
type Option func(*Client)

// nextTNS returns a fresh 16-bit transaction number. Zero is skipped
// because some SLC controllers treat TNS=0 specially.
func (c *Client) nextTNS() uint16 {
	for {
		n := uint16(c.tns.Add(1))
		if n != 0 {
			return n
		}
	}
}

// ---------------------------------------------------------------------------
// plc.PLC implementation
// ---------------------------------------------------------------------------

// Connect delegates to the underlying executor when it implements
// [Connector]; otherwise it is a no-op.
func (c *Client) Connect(ctx context.Context) error {
	if l, ok := c.exec.(Connector); ok {
		return l.Connect(ctx)
	}
	return nil
}

// Disconnect delegates to the underlying executor when it implements
// [Connector]; otherwise it is a no-op.
func (c *Client) Disconnect(ctx context.Context) error {
	if l, ok := c.exec.(Connector); ok {
		return l.Disconnect(ctx)
	}
	return nil
}

// IsConnected delegates to the underlying executor when it implements
// [Connector]; otherwise it returns true (no lifecycle to track).
func (c *Client) IsConnected() bool {
	if l, ok := c.exec.(Connector); ok {
		return l.IsConnected()
	}
	return true
}

// Read implements [plc.Reader]. Each [plc.DataPoint] must be a [File];
// other types cause an error. Results preserve input order.
func (c *Client) Read(ctx context.Context, points ...plc.DataPoint) ([]plc.Value, error) {
	out := make([]plc.Value, 0, len(points))
	for _, dp := range points {
		f, ok := dp.(File)
		if !ok {
			return nil, fmt.Errorf("pccc: expected File DataPoint, got %T", dp)
		}
		v, err := c.readFile(ctx, f)
		if err != nil {
			return nil, err
		}
		v.DataPoint = f
		out = append(out, v)
	}
	return out, nil
}

// Write implements [plc.Writer]. point must be a [File]; data must be in
// little-endian byte order matching the addressed file type (e.g. 2 bytes
// for an INT, 4 bytes for a REAL, or 6 bytes for a whole timer element).
// For bit addresses ("/N" or named bits like .EN), data[0] is treated as
// the boolean value and a read-modify-write cycle is performed.
func (c *Client) Write(ctx context.Context, point plc.DataPoint, data []byte) error {
	f, ok := point.(File)
	if !ok {
		return fmt.Errorf("pccc: expected File DataPoint, got %T", point)
	}
	a, err := f.parse()
	if err != nil {
		return err
	}
	if a.BitNum >= 0 {
		if len(data) == 0 {
			return fmt.Errorf("pccc: write to bit %s: empty data", f)
		}
		return c.writeBit(ctx, a, data[0] != 0)
	}
	return c.writeBytes(ctx, a, data)
}

// ---------------------------------------------------------------------------
// String-addressed convenience API
// ---------------------------------------------------------------------------

// ReadAddress reads a single value from the given SLC address. The returned
// [plc.Value] holds the raw little-endian bytes plus a type hint based on
// the file type:
//
//	N, B, S, I, O, D    -> plc.TypeInt16  (1 word, signed)
//	F                   -> plc.TypeFloat32
//	T, C, R sub-fields  -> plc.TypeInt16  (single word)
//	T, C, R whole       -> plc.TypeBytes  (6 bytes; use DecodeTimer/Counter)
//	bit suffix          -> plc.TypeBool   (1-byte: 0 or 1)
func (c *Client) ReadAddress(ctx context.Context, addr string) (plc.Value, error) {
	return c.readFile(ctx, File{Address: addr})
}

// ReadWords reads n consecutive 16-bit words starting at addr and returns
// them as a Go slice. Suitable for N, B, S, I, O, D files. The address must
// not contain a bit suffix or named sub-field; addressing a single sub-
// element of a multi-word T/C/R element with n>1 spans into the next word
// of the same element (PCCC byte-size semantics).
func (c *Client) ReadWords(ctx context.Context, addr string, n int) ([]int16, error) {
	if n < 1 {
		return nil, fmt.Errorf("pccc: ReadWords: n must be >= 1, got %d", n)
	}
	if n > 127 {
		return nil, fmt.Errorf("pccc: ReadWords: n must be <= 127, got %d", n)
	}
	a, err := ParseAddress(addr)
	if err != nil {
		return nil, err
	}
	if a.BitNum >= 0 {
		return nil, fmt.Errorf("pccc: ReadWords: bit suffix not supported in %q", addr)
	}
	raw, err := c.rawRead(ctx, a, n*2)
	if err != nil {
		return nil, err
	}
	if len(raw) < n*2 {
		return nil, fmt.Errorf("pccc: ReadWords: short reply (%d bytes, want %d)", len(raw), n*2)
	}
	out := make([]int16, n)
	for i := 0; i < n; i++ {
		out[i] = int16(binary.LittleEndian.Uint16(raw[i*2 : i*2+2]))
	}
	return out, nil
}

// WriteAddress writes a Go value to the addressed PCCC element. The Go
// type must match the file type:
//
//	N, B, S, I, O, D    -> int / int16 / uint16 / bool (bit suffix)
//	F                   -> float32 / float64
//	bit suffix or named -> bool
//
// Multi-element writes are not supported through this method — use
// WriteWords for arrays.
func (c *Client) WriteAddress(ctx context.Context, addr string, v any) error {
	a, err := ParseAddress(addr)
	if err != nil {
		return err
	}
	if a.BitNum >= 0 {
		b, ok := boolOf(v)
		if !ok {
			return fmt.Errorf("pccc: write to bit %s: value %T not coercible to bool", addr, v)
		}
		return c.writeBit(ctx, a, b)
	}
	switch a.FileType {
	case FileTypeFloat:
		f, ok := float32Of(v)
		if !ok {
			return fmt.Errorf("pccc: write to %s: value %T not coercible to float32", addr, v)
		}
		buf := make([]byte, 4)
		binary.LittleEndian.PutUint32(buf, math.Float32bits(f))
		return c.writeBytes(ctx, a, buf)
	default:
		n, ok := int16Of(v)
		if !ok {
			return fmt.Errorf("pccc: write to %s: value %T not coercible to int16", addr, v)
		}
		buf := make([]byte, 2)
		binary.LittleEndian.PutUint16(buf, uint16(n))
		return c.writeBytes(ctx, a, buf)
	}
}

// WriteWords writes a slice of 16-bit words starting at addr. Suitable for
// N, B, S, I, O, D files. The address must not contain a bit suffix.
func (c *Client) WriteWords(ctx context.Context, addr string, words []int16) error {
	if len(words) == 0 {
		return fmt.Errorf("pccc: WriteWords: empty slice")
	}
	if len(words) > 120 {
		return fmt.Errorf("pccc: WriteWords: too many words (%d, max 120)", len(words))
	}
	a, err := ParseAddress(addr)
	if err != nil {
		return err
	}
	if a.BitNum >= 0 {
		return fmt.Errorf("pccc: WriteWords: bit suffix not supported in %q", addr)
	}
	buf := make([]byte, len(words)*2)
	for i, w := range words {
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(w))
	}
	return c.writeBytes(ctx, a, buf)
}

// ---------------------------------------------------------------------------
// Internals
// ---------------------------------------------------------------------------

// readFile parses and reads a single File DataPoint into a plc.Value.
func (c *Client) readFile(ctx context.Context, f File) (plc.Value, error) {
	a, err := f.parse()
	if err != nil {
		return plc.Value{}, err
	}
	size := elementBytes(a, f.effectiveCount())
	raw, err := c.rawRead(ctx, a, size)
	if err != nil {
		return plc.Value{}, err
	}
	val := plc.Value{ByteOrder: plc.ByteOrderLittleEndian}
	switch {
	case a.BitNum >= 0:
		if len(raw) < 2 {
			return plc.Value{}, fmt.Errorf("pccc: bit read returned %d bytes", len(raw))
		}
		word := binary.LittleEndian.Uint16(raw[:2])
		bit := byte((word >> uint(a.BitNum)) & 1)
		val.Raw = []byte{bit}
		val.Type = plc.TypeBool
	case a.FileType == FileTypeFloat:
		val.Raw = clip(raw, 4)
		val.Type = plc.TypeFloat32
	case isTimerLike(a.FileType) && a.SubElement == 0:
		// Whole 6-byte element.
		val.Raw = clip(raw, 6)
		val.Type = plc.TypeBytes
	default:
		val.Raw = clip(raw, size)
		if f.effectiveCount() > 1 || size > 2 {
			val.Type = plc.TypeBytes
		} else {
			val.Type = plc.TypeInt16
		}
	}
	return val, nil
}

// rawRead encodes a typed-logical read of byteSize bytes at addr a and
// returns the raw data bytes from the PCCC reply.
func (c *Client) rawRead(ctx context.Context, a Address, byteSize int) ([]byte, error) {
	if byteSize < 1 || byteSize > 240 {
		return nil, fmt.Errorf("pccc: read size %d out of range [1,240]", byteSize)
	}
	tns := c.nextTNS()
	cmd, err := EncodeTypedRead(tns, byteSize, a.FileNumber, a.FileType, a.Element, a.SubElement)
	if err != nil {
		return nil, err
	}
	rawReply, err := c.exec.ExecutePCCC(ctx, cmd)
	if err != nil {
		return nil, err
	}
	reply, err := DecodeReply(rawReply)
	if err != nil {
		return nil, err
	}
	return reply.Data, nil
}

// writeBytes performs a typed-logical write of data at the addressed
// element/sub-element. The caller must ensure data is in the byte order
// and length expected by the file type.
func (c *Client) writeBytes(ctx context.Context, a Address, data []byte) error {
	tns := c.nextTNS()
	cmd, err := EncodeTypedWrite(tns, a.FileNumber, a.FileType, a.Element, a.SubElement, data)
	if err != nil {
		return err
	}
	rawReply, err := c.exec.ExecutePCCC(ctx, cmd)
	if err != nil {
		return err
	}
	_, err = DecodeReply(rawReply)
	return err
}

// writeBit performs a read-modify-write to set or clear a single bit in
// the addressed word. Most SLC firmware supports a dedicated bit-write
// command, but read-modify-write is portable across the family and never
// requires the caller to know the bit-write FNC code.
func (c *Client) writeBit(ctx context.Context, a Address, on bool) error {
	// Read the addressed word.
	wordAddr := Address{
		FileType:   a.FileType,
		FileNumber: a.FileNumber,
		Element:    a.Element,
		SubElement: a.SubElement,
		BitNum:     -1,
	}
	raw, err := c.rawRead(ctx, wordAddr, 2)
	if err != nil {
		return err
	}
	if len(raw) < 2 {
		return fmt.Errorf("pccc: write bit: short read (%d bytes)", len(raw))
	}
	word := binary.LittleEndian.Uint16(raw)
	mask := uint16(1) << uint(a.BitNum)
	if on {
		word |= mask
	} else {
		word &^= mask
	}
	buf := []byte{byte(word), byte(word >> 8)}
	return c.writeBytes(ctx, wordAddr, buf)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// elementBytes returns the read byte size for a parsed address with the
// given element count. Bit suffixes and timer/counter/control sub-fields
// are always single-word (2 bytes). Whole timer/counter/control elements
// are 6 bytes. Float files are 4 bytes per element. Everything else is 2
// bytes per element.
func elementBytes(a Address, count int) int {
	if count < 1 {
		count = 1
	}
	switch {
	case a.BitNum >= 0:
		return 2
	case a.FileType == FileTypeFloat:
		return 4 * count
	case isTimerLike(a.FileType):
		if a.SubElement > 0 {
			return 2 * count
		}
		return 6 * count
	default:
		return 2 * count
	}
}

func isTimerLike(ft FileType) bool {
	return ft == FileTypeTimer || ft == FileTypeCounter || ft == FileTypeControl
}

func clip(b []byte, n int) []byte {
	if len(b) <= n {
		out := make([]byte, len(b))
		copy(out, b)
		return out
	}
	out := make([]byte, n)
	copy(out, b[:n])
	return out
}

// boolOf coerces a Go value to bool. Accepts bool, integer (non-zero ==
// true), and byte.
func boolOf(v any) (bool, bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	case int:
		return x != 0, true
	case int16:
		return x != 0, true
	case int32:
		return x != 0, true
	case int64:
		return x != 0, true
	case uint:
		return x != 0, true
	case uint16:
		return x != 0, true
	case uint32:
		return x != 0, true
	case uint64:
		return x != 0, true
	case byte:
		return x != 0, true
	}
	return false, false
}

// int16Of coerces a Go value to int16 with range checking.
func int16Of(v any) (int16, bool) {
	switch x := v.(type) {
	case int16:
		return x, true
	case uint16:
		return int16(x), true
	case int:
		if x < math.MinInt16 || x > math.MaxInt16 {
			return 0, false
		}
		return int16(x), true
	case int32:
		if x < math.MinInt16 || x > math.MaxInt16 {
			return 0, false
		}
		return int16(x), true
	case int64:
		if x < math.MinInt16 || x > math.MaxInt16 {
			return 0, false
		}
		return int16(x), true
	case uint:
		if x > math.MaxInt16 {
			return 0, false
		}
		return int16(x), true
	case uint32:
		if x > math.MaxInt16 {
			return 0, false
		}
		return int16(x), true
	case uint64:
		if x > math.MaxInt16 {
			return 0, false
		}
		return int16(x), true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

// float32Of coerces a Go value to float32.
func float32Of(v any) (float32, bool) {
	switch x := v.(type) {
	case float32:
		return x, true
	case float64:
		return float32(x), true
	case int:
		return float32(x), true
	case int16:
		return float32(x), true
	case int32:
		return float32(x), true
	}
	return 0, false
}

// Compile-time interface checks.
var (
	_ plc.Reader = (*Client)(nil)
	_ plc.Writer = (*Client)(nil)
	_ plc.PLC    = (*Client)(nil)
)

// avoid "imported and not used" if the errors import becomes unused after
// future trimming.
var _ = errors.New
