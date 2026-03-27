# List Tags Example

Enumerate all tags on a Rockwell Logix controller using the CIP Symbol Object.

## What This Example Does

This program connects to a Logix controller (CompactLogix, ControlLogix, etc.)
over EtherNet/IP and lists every user-defined tag, displaying:

- **Instance ID** -- the CIP object instance number (1-based, with possible gaps)
- **Name** -- the symbolic tag name as it appears in Studio 5000
- **Type** -- the CIP data type code and its human-readable name

This is the same mechanism that HMI software and development tools use to
discover available tags on a controller.

## How CIP Symbol Enumeration Works

### The Symbol Object (Class 0x6B)

Logix controllers implement the CIP Symbol Object at class ID `0x6B`. Each
user-defined tag in the controller program is represented as an instance of this
class. The class itself (instance 0) provides metadata about the collection.

### Step 1: Query Class Attributes

The program first sends a **GetAttributeList** (service `0x03`) to the Symbol
class instance 0:

```
Path:    [Class 0x6B] [Instance 0]
Service: 0x03 (GetAttributeList)
Data:    [Count=2] [AttrID=1 (Revision)] [AttrID=2 (Max Instance)]
```

The response tells us:
- **Revision**: Symbol class revision (informational)
- **Max Instance**: The highest instance ID in use. We iterate from 1 to this
  value to discover all tags.

### Step 2: Iterate Instances

For each instance ID from 1 to Max Instance, the program sends another
GetAttributeList request:

```
Path:    [Class 0x6B] [Instance N]
Service: 0x03 (GetAttributeList)
Data:    [Count=2] [AttrID=1 (Name)] [AttrID=2 (Type)]
```

Possible responses:
- **Success (status 0x00)**: Returns the tag name (length-prefixed string) and
  type code (UINT).
- **Object does not exist (status 0x16)**: This instance was deleted. Skip it.
- **Path destination unknown (status 0x05)**: Instance does not exist. Skip it.

### Instance ID Gaps

When a tag is deleted from the PLC program, its instance ID is not immediately
reassigned. This creates gaps in the instance ID sequence. The enumeration
handles this gracefully by checking the CIP status code and skipping
non-existent instances.

### Type Code Interpretation

The type code returned for each tag is a 16-bit value:

- **Bits 0-14**: Base data type code
- **Bit 15**: Array flag (set if the tag is an array)

For example:
- `0x00C4` = DINT (scalar)
- `0x80C4` = DINT[] (array of DINT)
- `0x02A0` = STRUCT (user-defined type or built-in structured type)

### CIP Data Type Codes

| Code     | Name   | Go Type   | Size    |
|----------|--------|-----------|---------|
| `0x00C1` | BOOL   | `bool`    | 1 byte  |
| `0x00C2` | SINT   | `int8`    | 1 byte  |
| `0x00C3` | INT    | `int16`   | 2 bytes |
| `0x00C4` | DINT   | `int32`   | 4 bytes |
| `0x00C5` | LINT   | `int64`   | 8 bytes |
| `0x00C6` | USINT  | `uint8`   | 1 byte  |
| `0x00C7` | UINT   | `uint16`  | 2 bytes |
| `0x00C8` | UDINT  | `uint32`  | 4 bytes |
| `0x00C9` | ULINT  | `uint64`  | 8 bytes |
| `0x00CA` | REAL   | `float32` | 4 bytes |
| `0x00CB` | LREAL  | `float64` | 8 bytes |
| `0x00D0` | STRING | `string`  | variable |
| `0x02A0` | STRUCT | (varies)  | variable |

## EIP/CIP Protocol Flow

```
1. TCP connect to port 44818
2. EIP RegisterSession (0x0065)           -> session handle
3. EIP SendRRData (0x006F) wrapping CIP:
   - GetAttributeList on Class 0x6B, Instance 0  -> max instance
4. For each instance 1..max:
   - EIP SendRRData wrapping CIP:
     - GetAttributeList on Class 0x6B, Instance N  -> name, type
5. EIP UnregisterSession (0x0066)
6. TCP close
```

Each CIP request is wrapped in a Common Packet Format (CPF) with:
- Item 0: Null Address (type `0x0000`, length 0)
- Item 1: Unconnected Data (type `0x00B2`, contains CIP message)

## How to Run

```bash
go run ./examples/ethernetip/list_tags -addr 192.168.1.10:44818
```

## Expected Output

```
Connected to 192.168.1.10:44818
Enumerating tags via CIP Symbol Object (class 0x6B)...

Found 12 tags:

  Instance    Name                                      Type
  --------    ----                                      ----
  1           MyDINT                                    DINT (0x00C4)
  2           Temperature                               REAL (0x00CA)
  3           RunEnable                                 BOOL (0x00C1)
  5           PartCount                                 DINT (0x00C4)
  6           Timer_1                                   STRUCT (0x02A0)
  7           Counter_1                                 STRUCT (0x02A0)
  8           MyArray                                   DINT[] (0x80C4)
  10          Setpoint                                  REAL (0x00CA)
  11          MessageText                               STRING (0x00D0)
  12          BigCounter                                LINT (0x00C5)
  14          StatusWord                                DINT (0x00C4)
  15          OutputEnable                              BOOL (0x00C1)

Total: 12 tags

Done.
```

Note that instance IDs 4, 9, and 13 are missing -- these were previously
assigned to tags that have since been deleted from the PLC program.

## Common Errors

| Error | Cause | Fix |
|-------|-------|-----|
| `dial tcp ...: connection refused` | PLC unreachable | Verify IP, port 44818, firewall |
| `CIP Error: Status=0x08` | Service not supported -- controller does not support symbol enumeration | Some older firmware versions may not support GetAttributeList on class 0x6B |
| `context deadline exceeded` | Enumeration timed out | Controller may have thousands of tags; increase the timeout |
| `No tags found` | Controller has no user-defined tags | Create tags in Studio 5000 and download to the controller |

## Performance Considerations

The current implementation sends one CIP request per instance ID, which means
a controller with 1000 tags requires approximately 1000 round trips. At typical
LAN latencies (< 1 ms per round trip), this completes in a few seconds.

For controllers with very large tag databases (5000+), the enumeration may take
10-30 seconds. The 60-second context timeout in this example accommodates most
scenarios.

Some third-party implementations use CIP service `0x55`
(GetInstanceAttributeList) to fetch multiple instances in a single request, but
not all Logix firmware versions support this service on the Symbol class.

## Scope of Enumeration

The Symbol Object lists **controller-scoped** tags only. To enumerate tags in a
specific program scope (e.g. `MainProgram`), you would need to address the
program's Symbol Object via a more specific CIP path. This example covers the
most common use case of listing all top-level tags.
