# List Identity Example

Send EIP ListIdentity and ListServices commands to an EtherNet/IP device and
display the device's identity information and supported services.

## What This Example Does

This program connects to any EtherNet/IP device (PLC, I/O module, drive, etc.)
and queries its identity using two fundamental EIP encapsulation commands:

1. **ListIdentity** (command `0x0063`) -- returns the device's product name,
   vendor ID, serial number, firmware revision, and other identity fields from
   the CIP Identity Object.

2. **ListServices** (command `0x0004`) -- returns the communication services the
   device supports (typically "Communications" for CIP over TCP).

These commands are the EtherNet/IP equivalent of a network "ping" -- they
confirm the device is alive and speaking EIP, and provide useful diagnostic
information without reading any PLC tags.

## Protocols and Services Used

### EIP Encapsulation Layer

EtherNet/IP uses a 24-byte encapsulation header for all commands:

```
Offset  Size    Field
------  ------  ------------------------------------------
0-1     UINT    Command (e.g. 0x0063 for ListIdentity)
2-3     UINT    Length of data following header
4-7     UDINT   Session Handle (from RegisterSession)
8-11    UDINT   Status (0 = success)
12-19   8 bytes Sender Context (echoed back by device)
20-23   UDINT   Options (usually 0)
------  ------  ------------------------------------------
Total: 24 bytes
```

### EIP Command Reference

| Command | Code     | Description | Session Required? |
|---------|----------|-------------|-------------------|
| NOP     | `0x0000` | No operation | No |
| ListServices | `0x0004` | Query supported services | No |
| ListIdentity | `0x0063` | Query device identity | No |
| ListInterfaces | `0x0064` | Query network interfaces | No |
| RegisterSession | `0x0065` | Open a session (returns handle) | No (creates session) |
| UnregisterSession | `0x0066` | Close a session | Yes |
| SendRRData | `0x006F` | Send CIP request/reply | Yes |
| SendUnitData | `0x0070` | Send CIP connected data | Yes |

ListIdentity and ListServices are unique in that they do not require a
registered session. However, this example uses `ethernetip.Connect` which
registers a session as part of its standard handshake, and the commands work
identically within an active session.

### CIP Identity Object (Class 0x01)

The ListIdentity response contains data from the CIP Identity Object, which is
a mandatory object in every CIP device. Key attributes:

| Attribute | Type   | Description |
|-----------|--------|-------------|
| Vendor ID | UINT   | ODVA-assigned vendor identifier (1 = Rockwell) |
| Device Type | UINT | Device profile (0x0E = PLC, 0x0C = adapter) |
| Product Code | UINT | Vendor-specific product identifier |
| Revision | 2 bytes | Major.Minor firmware revision |
| Status | UINT | Device status word |
| Serial Number | UDINT | Unique serial number |
| Product Name | STRING | Human-readable name (max 32 chars) |
| State | USINT | Current device state |

### Common Vendor IDs

| ID  | Vendor |
|-----|--------|
| 1   | Rockwell Automation / Allen-Bradley |
| 9   | Schneider Electric |
| 47  | WAGO |
| 90  | Siemens |
| 283 | Beckhoff |

A full list is maintained by ODVA at https://www.odva.org.

### Common Device Types

| Code | Device Type |
|------|-------------|
| 0x00 | Generic Device |
| 0x02 | AC Drive |
| 0x06 | Photoelectric Sensor |
| 0x0C | Communications Adapter |
| 0x0E | Programmable Logic Controller |
| 0x10 | Position Controller |
| 0x13 | Safety Discrete I/O Device |
| 0x21 | CIP Motion Drive |
| 0x22 | CompoNet Repeater |

### ListServices Capability Flags

The capability flags in the ListServices response are a bitmask:

| Bit | Meaning |
|-----|---------|
| 5   | Supports CIP encapsulation via TCP |
| 8   | Supports CIP encapsulation via UDP |

Most Logix controllers set bit 5 (TCP support). UDP support (bit 8) is used
for implicit (I/O) messaging and ListIdentity broadcast discovery.

## How to Run

```bash
# Query a specific device
go run ./examples/ethernetip/list_identity -addr 192.168.1.10:44818

# Query a device on the default EIP port
go run ./examples/ethernetip/list_identity -addr 192.168.1.10:44818
```

## Expected Output

```
Connected to 192.168.1.10:44818

=== ListIdentity ===

  Product Name:   1756-L83E/B
  Vendor ID:      1
  Device Type:    14
  Product Code:   55
  Revision:       33.11
  Serial Number:  0x60ABCDEF
  Status:         0x0030
  State:          3
  Vendor:         Rockwell Automation / Allen-Bradley
  Device Class:   Programmable Logic Controller

=== ListServices ===

  Service Name:    Communications
  Type ID:         0x0100
  Version:         1
  Capabilities:    0x0120
    - Supports CIP encapsulation via TCP
    - Supports CIP encapsulation via UDP

Done.
```

## Common Errors

| Error | Cause | Fix |
|-------|-------|-----|
| `dial tcp ...: connection refused` | Device is not reachable on port 44818 | Verify IP address, check that EIP is enabled on the device |
| `dial tcp ...: i/o timeout` | Device is not responding | Check network cables, switch ports, IP configuration |
| `No identity items returned` | Device responded but with empty data | Uncommon; may indicate a non-standard EIP implementation |
| `context deadline exceeded` | Overall operation timed out | Check network connectivity |

## Use Cases

- **Network discovery**: Verify that an EIP device is online and identify its
  firmware version before attempting tag operations.
- **Inventory management**: Collect serial numbers, product codes, and firmware
  versions from all EIP devices on a network.
- **Troubleshooting**: Confirm that the correct device is at the expected IP
  address. The serial number is unique and never changes.
- **Firmware compatibility**: Check the firmware revision before using CIP
  features that may not be available on older firmware.

## Broadcast Discovery

In a production environment, you might want to discover all EIP devices on a
subnet by sending ListIdentity via UDP broadcast to port 44818. The
goindustrial library's current implementation uses TCP point-to-point
connections, so this example only queries a single device. UDP broadcast
discovery would require sending the ListIdentity command as a UDP datagram to
the subnet broadcast address (e.g. 192.168.1.255:44818).
