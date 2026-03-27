# Read Tag Example

Read one or more elements of a tag from a Rockwell Logix PLC using the
EtherNet/IP protocol and CIP Read Tag service.

## What This Example Does

This program demonstrates two ways to read tag data from an Allen-Bradley /
Rockwell Automation Logix controller (CompactLogix, ControlLogix, etc.):

1. **Raw byte read** -- `client.ReadTag` / `client.ReadTagElements` returns the
   CIP response as a `[]byte` including the 2-byte type code prefix. This is
   useful when you need to inspect the type at runtime or handle data manually.

2. **Generic typed read** -- `ethernetip.Read[int32]` strips the type header
   and unmarshals the data directly into a Go type. This is the most convenient
   approach when you already know the tag's data type.

## Protocols and Services Used

### EtherNet/IP (EIP)

EtherNet/IP is an application-layer protocol standardised by ODVA that carries
CIP messages over TCP/IP and UDP/IP. The default TCP port is **44818**.

Every EIP conversation begins with a **RegisterSession** handshake:

```
Client --> PLC:  RegisterSession (command 0x0065)
PLC    --> Client: RegisterSession reply (session handle = 0x00000001)
```

All subsequent requests include the session handle in the 24-byte EIP
encapsulation header.

### CIP (Common Industrial Protocol)

CIP is the object-oriented messaging protocol inside EIP. Each CIP request
targets an object addressed by a **path** (class / instance / attribute).
Logix controllers support **symbolic addressing**, which means you can specify a
tag name (e.g. `"MyDINT"`) directly as an ANSI string segment in the path
rather than needing to know numeric class/instance IDs.

The **Read Tag** service (CIP service code `0x4C`) returns the tag's data type
followed by the raw value bytes:

```
Response: [TypeCode (2 bytes LE)] [Data (N bytes)]
```

For structured types (type code >= `0x02A0`), an additional 2-byte structure
handle follows the type code, making a 4-byte header.

### CIP Data Types

| Code     | CIP Name | Go Type   | Size    |
|----------|----------|-----------|---------|
| `0x00C1` | BOOL     | `bool`    | 1 byte  |
| `0x00C2` | SINT     | `int8`    | 1 byte  |
| `0x00C3` | INT      | `int16`   | 2 bytes |
| `0x00C4` | DINT     | `int32`   | 4 bytes |
| `0x00C5` | LINT     | `int64`   | 8 bytes |
| `0x00C6` | USINT    | `uint8`   | 1 byte  |
| `0x00C7` | UINT     | `uint16`  | 2 bytes |
| `0x00C8` | UDINT    | `uint32`  | 4 bytes |
| `0x00C9` | ULINT    | `uint64`  | 8 bytes |
| `0x00CA` | REAL     | `float32` | 4 bytes |
| `0x00CB` | LREAL    | `float64` | 8 bytes |
| `0x00D0` | STRING   | `string`  | variable |
| `0x02A0` | STRUCT   | (varies)  | variable |

## How to Run

```bash
# Read a single DINT tag
go run ./examples/ethernetip/read_tag -addr 192.168.1.10:44818 -tag MyDINT

# Read 10 elements of an array tag
go run ./examples/ethernetip/read_tag -addr 192.168.1.10:44818 -tag MyArray -count 10

# If the PLC is on the default EIP port you can omit the port
go run ./examples/ethernetip/read_tag -addr 192.168.1.10:44818 -tag Temperature
```

## Expected Output

```
Connected to 192.168.1.10:44818

--- Raw byte read ---
Tag:        MyDINT
Type Code:  0x00C4 (DINT)
Raw bytes:  C4 00 2A 00 00 00
Data bytes: 2A 00 00 00

--- Typed read (DINT / int32) ---
Value (int32): 42

Done.
```

For an array read with `-count 5`:

```
Connected to 192.168.1.10:44818

--- Raw byte read ---
Tag:        MyArray
Type Code:  0x00C4 (DINT)
Raw bytes:  C4 00 01 00 00 00 02 00 00 00 03 00 00 00 04 00 00 00 05 00 00 00
Data bytes: 01 00 00 00 02 00 00 00 03 00 00 00 04 00 00 00 05 00 00 00

--- Typed read (DINT / int32) ---
Values ([]int32): [1 2 3 4 5]

Done.
```

## Common Errors

| Error | Cause | Fix |
|-------|-------|-----|
| `dial tcp ...: connection refused` | PLC is not reachable or EIP port is blocked | Verify IP address, check firewall rules, confirm port 44818 is open |
| `CIP Error: Status=0x05` | Path destination unknown -- tag name does not exist | Double-check the tag name in the PLC program; names are case-sensitive |
| `CIP Error: Status=0x13` | Not enough data -- requested more elements than the array holds | Reduce `-count` to match the actual array size |
| `CIP Error: Status=0x04` | Path segment error -- malformed tag path | Check for typos or unsupported characters in the tag name |
| `context deadline exceeded` | Operation timed out | Increase timeout, check network connectivity |
| `response too short` | PLC returned an unexpectedly small response | May indicate a firmware bug or protocol mismatch; capture a Wireshark trace |

## Symbolic Addressing in Detail

Logix controllers resolve tag names through the **Symbol Object** (CIP class
`0x6B`). When you send a Read Tag request with a symbolic path like `"MyDINT"`,
the controller:

1. Looks up `"MyDINT"` in its internal symbol table.
2. Resolves it to a memory address and data type.
3. Returns the data with the type code prepended.

You can address nested members using dot notation (`MyUDT.Field1`) and array
elements using bracket notation (`MyArray[5]`). The symbolic segment in the CIP
path encodes the name as a length-prefixed ANSI string, padded to an even
number of bytes.
