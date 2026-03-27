# Read Tag Typed Example

Demonstrate type-safe generic reads using `ethernetip.Read[T]`,
`ethernetip.ReadSlice[T]`, and `client.ReadTagInto` for struct unmarshaling.

## What This Example Does

This program reads a tag from a Rockwell Logix PLC and deserializes the CIP
response directly into a native Go type using Go generics. It supports every
common CIP atomic type plus structured types (Timer, Counter) via the
`cip.Unmarshaler` interface.

For scalar reads it calls:

```go
val, err := ethernetip.Read[int32](client, ctx, "MyDINT")
```

For array reads it calls:

```go
vals, err := ethernetip.ReadSlice[int32](client, ctx, "MyArray", 10)
```

For structured types it uses `ReadTagInto`:

```go
var timer cip.Timer
err := client.ReadTagInto(ctx, "MyTimer", &timer)
```

## How the Generic Helpers Work

### ethernetip.Read[T]

1. Sends a CIP Read Tag request (service `0x4C`) for 1 element.
2. Receives the response: `[TypeCode 2B] [Data NB]` (or `[TypeCode 2B] [StructHandle 2B] [Data NB]` for structs).
3. Determines header length: 2 bytes for atomic types (code < `0x02A0`), 4 bytes for structs.
4. Strips the header and calls `cip.Unmarshal(data, &result)`.
5. `cip.Unmarshal` checks if `T` implements `cip.Unmarshaler` (custom decode) or falls back to `binary.Read` (little-endian).

### ethernetip.ReadSlice[T]

Same as `Read[T]` but requests `count` elements. The response data contains
`count` consecutive values packed contiguously after the type header.

### client.ReadTagInto

Reads a tag and unmarshals the data into any pointer type. This is especially
useful for structured types like `cip.Timer` and `cip.Counter` that implement
the `cip.Unmarshaler` interface with custom bit-field decoding.

## Protocols and Services Used

### CIP Read Tag (Service 0x4C)

The read request contains:
- Service code: `0x4C`
- Path: symbolic segment encoding the tag name as an ANSI string
- Request data: 2-byte element count (little-endian UINT)

The response contains:
- 2-byte CIP data type code (e.g. `0x00C4` for DINT)
- For structs: additional 2-byte structure handle
- N bytes of tag data

### CIP Data Type to Go Type Mapping

| CIP Type | Code     | Go Type    | Read Call |
|----------|----------|------------|-----------|
| BOOL     | `0x00C1` | `bool`     | `Read[bool]` |
| SINT     | `0x00C2` | `int8`     | `Read[int8]` |
| INT      | `0x00C3` | `int16`    | `Read[int16]` |
| DINT     | `0x00C4` | `int32`    | `Read[int32]` |
| LINT     | `0x00C5` | `int64`    | `Read[int64]` |
| USINT    | `0x00C6` | `uint8`    | `Read[uint8]` |
| UINT     | `0x00C7` | `uint16`   | `Read[uint16]` |
| UDINT    | `0x00C8` | `uint32`   | `Read[uint32]` |
| ULINT    | `0x00C9` | `uint64`   | `Read[uint64]` |
| REAL     | `0x00CA` | `float32`  | `Read[float32]` |
| LREAL    | `0x00CB` | `float64`  | `Read[float64]` |
| TIMER    | `0x02A0+`| `cip.Timer`| `ReadTagInto` |
| COUNTER  | `0x02A0+`| `cip.Counter`| `ReadTagInto` |

### cip.Unmarshaler Interface

Structured types implement this interface to provide custom decoding:

```go
type Unmarshaler interface {
    UnmarshalCIP(data []byte) error
}
```

`cip.Timer` and `cip.Counter` implement `UnmarshalCIP` to decode the Rockwell
14-byte structure layout (2-byte reserved + 4-byte status bits + 4-byte PRE +
4-byte ACC) and extract individual boolean flags from the status DINT.

## How to Run

```bash
# Read a DINT (int32)
go run ./examples/ethernetip/read_tag_typed \
  -addr 192.168.1.10:44818 -tag MyDINT -type DINT

# Read a REAL (float32)
go run ./examples/ethernetip/read_tag_typed \
  -addr 192.168.1.10:44818 -tag Temperature -type REAL

# Read 10 elements of a DINT array
go run ./examples/ethernetip/read_tag_typed \
  -addr 192.168.1.10:44818 -tag MyArray -type DINT -count 10

# Read a BOOL
go run ./examples/ethernetip/read_tag_typed \
  -addr 192.168.1.10:44818 -tag RunEnable -type BOOL

# Read each integer type
go run ./examples/ethernetip/read_tag_typed -addr 192.168.1.10:44818 -tag MySINT -type SINT
go run ./examples/ethernetip/read_tag_typed -addr 192.168.1.10:44818 -tag MyINT -type INT
go run ./examples/ethernetip/read_tag_typed -addr 192.168.1.10:44818 -tag MyLINT -type LINT
go run ./examples/ethernetip/read_tag_typed -addr 192.168.1.10:44818 -tag MyUSINT -type USINT
go run ./examples/ethernetip/read_tag_typed -addr 192.168.1.10:44818 -tag MyUINT -type UINT
go run ./examples/ethernetip/read_tag_typed -addr 192.168.1.10:44818 -tag MyUDINT -type UDINT
go run ./examples/ethernetip/read_tag_typed -addr 192.168.1.10:44818 -tag MyULINT -type ULINT

# Read a Timer structured tag
go run ./examples/ethernetip/read_tag_typed \
  -addr 192.168.1.10:44818 -tag Timer_1 -type TIMER

# Read a Counter structured tag
go run ./examples/ethernetip/read_tag_typed \
  -addr 192.168.1.10:44818 -tag Counter_1 -type COUNTER
```

## Expected Output

For a DINT scalar:

```
Connected to 192.168.1.10:44818

DINT value: 42

Done.
```

For a DINT array with `-count 5`:

```
Connected to 192.168.1.10:44818

DINT values: [1 2 3 4 5]

Done.
```

For a TIMER:

```
Connected to 192.168.1.10:44818

--- Using ReadTagInto for Timer ---
Timer:
  PRE (preset):      5000 ms
  ACC (accumulated):  3200 ms
  EN  (enable):       true
  TT  (timer timing): true
  DN  (done):         false

Done.
```

## Common Errors

| Error | Cause | Fix |
|-------|-------|-----|
| `binary.Read: invalid type` | Go type T does not match the tag's byte size | Ensure -type matches the PLC tag's actual type |
| `CIP Error: Status=0x05` | Tag not found | Verify tag name |
| `CIP Error: Status=0x13` | Requested more elements than array holds | Reduce -count |
| `response too short` | PLC returned fewer bytes than expected | Tag may be a different type than specified |
| `insufficient data for Timer` | Tag is not actually a Timer struct | Verify the tag is a TON/TOF/RTO instruction output |

## Type Safety Considerations

The generic type parameter `T` is checked at compile time, but there is no
runtime validation that `T` matches the CIP type code in the response. If you
call `Read[float32]` on a DINT tag, you will get the IEEE 754 reinterpretation
of the integer bytes (not a conversion). Always match the Go type to the CIP
type:

- DINT tag -> `Read[int32]` (correct)
- DINT tag -> `Read[float32]` (compiles but gives wrong values)
- DINT tag -> `Read[int64]` (may fail: not enough bytes for 8-byte read)
