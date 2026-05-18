package cip

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseTagPath converts a Logix-style tag string into a CIP EPATH composed of
// ANSI Extended Symbol segments and (where applicable) Member ID segments for
// array indices and BOOL bit access.
//
// Supported syntax:
//
//	"MyTag"                              // simple controller-scoped tag
//	"MyStruct.Field"                     // struct member access
//	"MyArray[5]"                         // 1D array element
//	"Matrix[2,3]"                        // 2D array element
//	"Cube[1,2,3]"                        // 3D array element
//	"Program:MainProgram.MyTag"          // program-scoped tag
//	"Program:Foo.Bar[5].Baz"             // program scope + array + member
//	"Local:2:I.Data[0]"                  // module I/O tag with array element
//	"MyDINT.5"                           // bit access on an integer (BOOL bit 5)
//
// Only "." and "[...]" act as segment separators. Colons remain inside the
// symbol because they appear in program-scope prefixes ("Program:Name") and
// module I/O tag names ("Local:2:I"). A purely numeric token after "." is
// encoded as a Member ID segment (used for BOOL bit access); a token containing
// any non-digit character is encoded as a symbol. Array indices inside "[...]"
// are always encoded as Member ID segments.
//
// On invalid syntax, ParseTagPath returns an error and an empty Path.
func ParseTagPath(tag string) (Path, error) {
	if tag == "" {
		return nil, fmt.Errorf("cip: empty tag path")
	}

	parts, err := splitTagSegments(tag)
	if err != nil {
		return nil, err
	}

	p := NewPath()
	for i, part := range parts {
		name, indices, err := parseTagSegment(part)
		if err != nil {
			return nil, fmt.Errorf("cip: tag %q: %w", tag, err)
		}
		if name == "" && len(indices) == 0 {
			return nil, fmt.Errorf("cip: tag %q: empty segment", tag)
		}
		if name != "" {
			if i > 0 && isAllDigits(name) {
				// Bit access on a previous symbol, e.g. "MyDINT.5" -> Member(5).
				n, err := strconv.ParseUint(name, 10, 32)
				if err != nil {
					return nil, fmt.Errorf("cip: tag %q: invalid numeric member %q", tag, name)
				}
				addMember32(&p, uint32(n))
			} else {
				p.AddSymbolicSegment(name)
			}
		}
		for _, idx := range indices {
			addMember32(&p, idx)
		}
	}
	return p, nil
}

// splitTagSegments splits a tag path on '.' while keeping '[...]' groups intact.
// Returns one entry per dotted segment; bracket groups stay attached to the
// preceding symbol (e.g. "Foo[1].Bar" -> ["Foo[1]", "Bar"]).
func splitTagSegments(tag string) ([]string, error) {
	var (
		parts []string
		start int
		depth int
	)
	for i := 0; i < len(tag); i++ {
		switch tag[i] {
		case '[':
			depth++
		case ']':
			if depth == 0 {
				return nil, fmt.Errorf("cip: tag %q: unmatched ']'", tag)
			}
			depth--
		case '.':
			if depth == 0 {
				parts = append(parts, tag[start:i])
				start = i + 1
			}
		}
	}
	if depth != 0 {
		return nil, fmt.Errorf("cip: tag %q: unmatched '['", tag)
	}
	parts = append(parts, tag[start:])
	return parts, nil
}

// parseTagSegment splits "Name[i,j,...]" into its name and a list of indices.
// A bare "Name" returns name + nil indices. A bare "[i]" (no name) is allowed
// only as a follow-on segment but is uncommon; we return name="" + indices.
func parseTagSegment(seg string) (name string, indices []uint32, err error) {
	open := strings.IndexByte(seg, '[')
	if open < 0 {
		return seg, nil, nil
	}
	name = seg[:open]
	if !strings.HasSuffix(seg, "]") {
		return "", nil, fmt.Errorf("segment %q: bracket group not closed", seg)
	}
	inner := seg[open+1 : len(seg)-1]
	if inner == "" {
		return "", nil, fmt.Errorf("segment %q: empty index", seg)
	}
	for part := range strings.SplitSeq(inner, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return "", nil, fmt.Errorf("segment %q: empty index value", seg)
		}
		n, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return "", nil, fmt.Errorf("segment %q: invalid index %q", seg, part)
		}
		indices = append(indices, uint32(n))
	}
	return name, indices, nil
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// addMember32 appends a Member ID segment, choosing 8-, 16-, or 32-bit
// encoding based on the value's magnitude. This is the CIP encoding Rockwell
// Logix controllers expect for array indices and BOOL bit access.
func addMember32(p *Path, id uint32) {
	switch {
	case id <= 0xFF:
		*p = append(*p, SegmentTypeLogical|LogicalTypeMember|LogicalFormat8Bit, byte(id))
	case id <= 0xFFFF:
		*p = append(*p,
			SegmentTypeLogical|LogicalTypeMember|LogicalFormat16Bit, 0x00,
			byte(id), byte(id>>8))
	default:
		*p = append(*p,
			SegmentTypeLogical|LogicalTypeMember|LogicalFormat32Bit, 0x00,
			byte(id), byte(id>>8), byte(id>>16), byte(id>>24))
	}
}
