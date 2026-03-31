# Custom CIP Type

Demonstrates how to register a custom CIP struct type with the type registry
so that vendor-specific or site-specific UDT/AOI types can be decoded, encoded,
and displayed by name.

## Problem

When listing tags on a Rockwell Logix controller, vendor-specific and
user-defined types appear as `UNKNOWN(0x…)`:

```
  432         MyCustomTimer                            UNKNOWN(0x2F83)[] (0xAF83)
```

## Solution

Implement the `cip.TypeCodec` interface on a Go struct and register it with
`cip.RegisterType`. After registration:

- `cip.DataType(0x2F83).String()` returns `"SET_ON_3_TMR"` instead of `"UNKNOWN(0x2F83)"`
- `cip.LookupType(0x2F83)` returns a ready-to-use codec for decoding/encoding
- `cip.GoTypeToCIPType(&myStruct)` returns the correct CIP DataType for writes

### TypeCodec interface

```go
type TypeCodec interface {
    Marshaler                    // MarshalCIP() ([]byte, error)
    Unmarshaler                  // UnmarshalCIP(data []byte) error
    CIPType() cip.DataType       // The CIP type code (without array flag)
}
```

Optionally implement `fmt.Stringer` for human-readable display in
`DataType.String()`.

### Registration

```go
func init() {
    cip.RegisterType(0x2F83, func() cip.TypeCodec {
        return new(SetOn3Timer)
    })
}
```

Registration must happen at init() time. The registry is not concurrency-safe
for writes.

## Running

Offline demo (no PLC required):

```bash
go run .
```

With a real PLC:

```bash
go run . -addr 192.168.1.10:44818 -tag MyCustomTimer
```

## Output

```
=== Custom CIP Type Registry Demo ===

DataType(0x2F83).String() = SET_ON_3_TMR
DataType(0xAF83).String() = SET_ON_3_TMR[]  (array variant)

LookupType(0x2F83) -> *main.SetOn3Timer

Encoded wire bytes (12 bytes):

>>> WRITE 12 bytes
00000000  00 00 00 d0 40 1f 00 00  00 00 00 00             |....@.......    |

Decoded fields:
  PRE: 8000 ms  (8.0 s)
  ACC: 0 ms
  Progress: 0.0%

  EN:  true
  EN2: true
  EN3: false
  TT:  true
  DN:  false

GoTypeToCIPType(*SetOn3Timer) = SET_ON_3_TMR (0x2F83)
```

## Vendor packages

For well-known Rockwell types (Timer, Counter, PID, Control), use the
`vendor/rockwell` package instead of defining your own:

```go
import "github.com/iceisfun/goindustrial/protocol/ethernetip/vendor/rockwell"

func init() {
    // Register with type codes discovered from your controller via ListTags
    rockwell.RegisterTimer(0x02B3)
    rockwell.RegisterCounter(0x02B4)
    rockwell.RegisterPID(0x02B5)
    rockwell.RegisterControl(0x02B6)
}
```
