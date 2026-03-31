package cip

import "fmt"

// TypeCodec is the interface implemented by types that can be registered in the
// CIP type registry for automatic encoding, decoding, and name resolution.
//
// A TypeCodec combines [Marshaler] and [Unmarshaler] with a type code accessor.
// Types that also implement [fmt.Stringer] will have their String() result used
// by [DataType.String] for human-readable display instead of "UNKNOWN(0x…)".
//
// Registration is intended for init() time only — the registry is not
// concurrency-safe for writes.
//
//	func init() {
//	    cip.RegisterType(0x2F83, func() cip.TypeCodec { return new(SetOn3Timer) })
//	}
type TypeCodec interface {
	Marshaler
	Unmarshaler
	// CIPType returns the CIP DataType code for this struct (without the
	// array flag). This must match the code used with [RegisterType].
	CIPType() DataType
}

// typeEntry holds a registered type's factory and optional display name.
type typeEntry struct {
	factory func() TypeCodec
	name    string // from fmt.Stringer if implemented, else ""
}

// registry maps base DataType codes to their registered TypeCodec factories.
// Populated at init() time only — no synchronisation required.
var registry = map[DataType]typeEntry{}

// RegisterType registers a factory for a custom CIP struct type identified by
// dt (the base type code, without the array flag). The factory must return a
// new zero-value pointer implementing [TypeCodec].
//
// If the returned TypeCodec also implements [fmt.Stringer], the String() result
// is used by [DataType.String] for human-readable display.
//
// RegisterType is intended to be called from init() functions. It panics if dt
// has already been registered.
//
//	cip.RegisterType(0x2F83, func() cip.TypeCodec { return new(SetOn3Timer) })
func RegisterType(dt DataType, factory func() TypeCodec) {
	base := dt.Base()
	if _, dup := registry[base]; dup {
		panic(fmt.Sprintf("cip: duplicate RegisterType for DataType 0x%04X", uint16(base)))
	}
	// Probe the factory once to extract a display name from Stringer.
	var name string
	probe := factory()
	if s, ok := probe.(fmt.Stringer); ok {
		name = s.String()
	}
	registry[base] = typeEntry{factory: factory, name: name}
}

// LookupType returns a new zero-value TypeCodec for the given DataType, or nil
// if no type has been registered for that code. The array flag is masked off
// before lookup.
func LookupType(dt DataType) TypeCodec {
	e, ok := registry[dt.Base()]
	if !ok {
		return nil
	}
	return e.factory()
}

// lookupTypeName returns the registered display name for dt, or "" if none.
func lookupTypeName(dt DataType) string {
	e, ok := registry[dt.Base()]
	if !ok {
		return ""
	}
	return e.name
}
