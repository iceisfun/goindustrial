# Write Tag Example

Write a value to a tag on a Rockwell Logix PLC using the EtherNet/IP protocol
and CIP Write Tag service, then read it back to verify.

## What This Example Does

This program:

1. Connects to a Logix controller over EtherNet/IP (TCP port 44818).
2. Parses a user-supplied value string into the correct Go type based on the
   specified CIP type name.
3. Writes the value to the specified tag using `client.WriteTag`.
4. Reads the tag back using `client.ReadTag` and displays the result.

It supports all common CIP atomic data types: BOOL, SINT, INT, DINT, LINT,
USINT, UINT, UDINT, ULINT, REAL, LREAL, and STRING.

## Protocols and Services Used

### CIP Write Tag (Service 0x4D)

The Write Tag service sends data to a tag on the controller. The request
contains:

```
[Service Code: 0x4D]
[Path: symbolic segment with tag name]
[Data Type: 2-byte CIP type code]
[Element Count: 2-byte UINT, always 1 for scalar writes]
[Data: N bytes, little-endian]
```

The PLC validates the type code against the tag's actual type. If they do not
match, the PLC returns CIP general status `0x04` (path segment error) or
`0x20` (invalid parameter).

### Type Auto-Detection

The `WriteTag` method uses `cip.GoTypeToCIPType` to automatically determine the
CIP data type from the Go value's type. For example:

- `int32` maps to DINT (0x00C4)
- `float32` maps to REAL (0x00CA)
- `bool` maps to BOOL (0x00C1)
- `string` maps to STRING (0x00D0)

This means you must pass the correct Go type -- passing an `int64` when the PLC
tag is a DINT will result in the wrong type code being sent.

### EIP Session Lifecycle

```
1. TCP connect to port 44818
2. RegisterSession (0x0065) -> session handle
3. SendRRData (0x006F) wrapping CIP Write Tag request
4. SendRRData (0x006F) wrapping CIP Read Tag request (verify)
5. UnregisterSession (0x0066)
6. TCP close
```

All CIP requests are wrapped in SendRRData, which stands for "Send Request/
Reply Data". This is the standard EIP command for unconnected CIP messaging.

### CIP Data Type Reference

| Type Name | Code     | Go Type   | Size     | Value Range |
|-----------|----------|-----------|----------|-------------|
| BOOL      | `0x00C1` | `bool`    | 1 byte   | true / false |
| SINT      | `0x00C2` | `int8`    | 1 byte   | -128 to 127 |
| INT       | `0x00C3` | `int16`   | 2 bytes  | -32768 to 32767 |
| DINT      | `0x00C4` | `int32`   | 4 bytes  | -2^31 to 2^31-1 |
| LINT      | `0x00C5` | `int64`   | 8 bytes  | -2^63 to 2^63-1 |
| USINT     | `0x00C6` | `uint8`   | 1 byte   | 0 to 255 |
| UINT      | `0x00C7` | `uint16`  | 2 bytes  | 0 to 65535 |
| UDINT     | `0x00C8` | `uint32`  | 4 bytes  | 0 to 2^32-1 |
| ULINT     | `0x00C9` | `uint64`  | 8 bytes  | 0 to 2^64-1 |
| REAL      | `0x00CA` | `float32` | 4 bytes  | IEEE 754 single |
| LREAL     | `0x00CB` | `float64` | 8 bytes  | IEEE 754 double |
| STRING    | `0x00D0` | `string`  | variable | Up to 82 chars (Logix default) |

## How to Run

```bash
# Write a DINT (32-bit integer)
go run ./examples/ethernetip/write_tag \
  -addr 192.168.1.10:44818 -tag MyDINT -type DINT -value 42

# Write a REAL (32-bit float)
go run ./examples/ethernetip/write_tag \
  -addr 192.168.1.10:44818 -tag Temperature -type REAL -value 98.6

# Write a BOOL
go run ./examples/ethernetip/write_tag \
  -addr 192.168.1.10:44818 -tag RunEnable -type BOOL -value true

# Write a STRING
go run ./examples/ethernetip/write_tag \
  -addr 192.168.1.10:44818 -tag MessageText -type STRING -value "Hello PLC"

# Write a LINT (64-bit integer)
go run ./examples/ethernetip/write_tag \
  -addr 192.168.1.10:44818 -tag BigCounter -type LINT -value 1234567890123

# Write a signed 8-bit integer
go run ./examples/ethernetip/write_tag \
  -addr 192.168.1.10:44818 -tag SmallVal -type SINT -value -42
```

## Expected Output

```
Connected to 192.168.1.10:44818

Writing tag "MyDINT" = 42 (type DINT)
Write successful.

--- Read-back verification ---
Type Code:  0x00C4 (DINT)
Raw bytes:  C4 00 2A 00 00 00
Read-back:  42

Done.
```

## Common Errors

| Error | Cause | Fix |
|-------|-------|-----|
| `CIP Error: Status=0x05` | Tag does not exist on the PLC | Verify the tag name matches the controller program |
| `CIP Error: Status=0x04` | Path segment error (bad tag name) | Check for typos or invalid characters |
| `CIP Error: Status=0x10` | Privilege violation -- tag may be read-only | Check tag external access settings in Studio 5000 |
| `CIP Error: Status=0x09` | Invalid attribute value -- type mismatch or out of range | Ensure -type matches the PLC tag type; check value range |
| `unsupported CIP type` | The -type flag is not one of the supported names | Use one of: BOOL, SINT, INT, DINT, LINT, USINT, UINT, UDINT, ULINT, REAL, LREAL, STRING |
| `strconv.ParseInt: parsing "abc"` | The -value cannot be parsed as the given numeric type | Provide a valid number for numeric types |
| `dial tcp ...: connection refused` | PLC unreachable | Check IP, port 44818, firewall |

## Important Notes

- **Tag names are case-sensitive** on Logix controllers. `myDint` and `MyDINT`
  are different tags.
- **Type must match.** If the PLC tag is a DINT but you specify `-type REAL`,
  the PLC will reject the write because the type code in the request will not
  match the tag's actual type.
- **Online edits.** Be careful writing to tags that are actively used by the PLC
  program. Changing a setpoint or enable flag while the process is running can
  have real-world consequences.
- **STRING length.** The default Logix STRING type holds up to 82 characters. If
  your string exceeds this limit, the PLC will reject the write.
