# Device Identification Example

Demonstrates reading device identification data from a Modbus TCP server using function code 0x2B with MEI type 0x0E (Read Device Identification).

## What It Does

This example connects to a Modbus TCP server and requests the device's identification information, including:

- **Vendor Name** -- the manufacturer of the device
- **Product Code** -- a unique product identifier
- **Major/Minor Revision** -- the firmware or software version

It also displays the device's conformity level and handles the "More Follows" pagination mechanism for devices with many identification objects.

## Modbus Concepts

### Modbus Encapsulated Interface (MEI)

Function code 0x2B is the MEI Transport function, which provides a generic encapsulation mechanism for sub-functions. The Read Device Identification sub-function (MEI type 0x0E) is the most commonly used MEI operation. Defined in Section 6.21 of the Modbus specification.

### Device Identification Objects

The Modbus specification defines a hierarchy of identification objects:

| Object ID | Name                   | Category | Mandatory |
|-----------|------------------------|----------|-----------|
| 0x00      | VendorName             | Basic    | Yes       |
| 0x01      | ProductCode            | Basic    | Yes       |
| 0x02      | MajorMinorRevision     | Basic    | Yes       |
| 0x03      | VendorURL              | Regular  | No        |
| 0x04      | ProductName            | Regular  | No        |
| 0x05      | ModelName              | Regular  | No        |
| 0x06      | UserApplicationName    | Regular  | No        |
| 0x80-0xFF | Vendor-specific        | Extended | No        |

### Read Device ID Codes

The request includes a "Read Device ID Code" that determines which objects to return:

- **0x01 (Basic Stream)** -- returns the three mandatory objects
- **0x02 (Regular Stream)** -- returns mandatory + standard optional objects
- **0x03 (Extended Stream)** -- returns all objects including vendor-specific
- **0x04 (Specific Object)** -- returns a single object by its ID

### Conformity Level

The response includes a conformity level indicating what identification categories the device supports:

- **Basic** (0x01/0x81) -- only mandatory objects (VendorName, ProductCode, Revision)
- **Regular** (0x02/0x82) -- adds standard optional objects (URL, ProductName, etc.)
- **Extended** (0x03/0x83) -- adds vendor-specific objects

Values with bit 7 set (0x81, 0x82, 0x83) indicate that individual object access (code 0x04) is also supported.

### More Follows / Pagination

If the device has more objects than fit in a single response, the `MoreFollows` field is set to `0xFF` and `NextObjectID` indicates where to resume. The client should issue another request starting from that object ID.

## How to Run

```bash
# Read device identification from localhost
go run ./examples/modbus/device_identification/ -addr 127.0.0.1 -port 502

# Read from a remote device
go run ./examples/modbus/device_identification/ -addr 192.168.1.100

# Read from a specific unit behind a gateway
go run ./examples/modbus/device_identification/ -addr 192.168.1.1 -unit 5

# Read from a simulator on a non-standard port
go run ./examples/modbus/device_identification/ -addr 127.0.0.1 -port 5020
```

### Flags

| Flag    | Default     | Description                           |
|---------|-------------|---------------------------------------|
| `-addr` | `127.0.0.1` | Modbus TCP server address             |
| `-port` | `502`       | Modbus TCP port                       |
| `-unit` | `1`         | Modbus unit ID (slave address, 0-247) |

## Expected Output

```
Connecting to Modbus TCP server at 127.0.0.1:502 (unit ID 1)...
Connected successfully.

--- Reading Device Identification (FC 0x2B/0x0E) ---

Requesting basic identification (mandatory objects)...

  Response Metadata:
    Conformity Level: Basic (stream+individual)
    More Follows:     No
    Next Object ID:   0x00
    Number of Objects: 3

  Device Identification Objects:
    ID     Name                      Value
    ------ ------------------------- -----
    0x00   VendorName                Acme Industrial
    0x01   ProductCode               ACI-9000
    0x02   MajorMinorRevision        V2.1.0

  Parsed Fields:
    Vendor Name: Acme Industrial
    Product Code: ACI-9000
    Revision: V2.1.0

Done.
```

## Common Errors and Troubleshooting

| Error | Cause | Fix |
|-------|-------|-----|
| `Modbus exception: Function Code Not Supported (0x01)` | Device does not implement FC 0x2B/0x0E | This is expected for simpler devices; device identification is optional |
| `Modbus exception: Invalid Data Value (0x03)` | Unsupported Read Device ID code or object ID | Try using Basic Stream (0x01) starting from object 0x00 |
| `connection refused` | No server at the specified address | Verify server address and port |
| `Number of Objects: 0` | Server returned an empty response | Some simulators have minimal identification support |

## Specification References

- Modbus Application Protocol V1.1b3, Section 6.21 -- Read Device Identification (FC 0x2B/0x0E)
- Modbus Application Protocol V1.1b3, Section 6.21, Table 72 -- Object ID Codes
- Modbus Application Protocol V1.1b3, Section 6.21, Table 73 -- Read Device ID Codes
- Modbus Application Protocol V1.1b3, Section 6.21, Table 74 -- Conformity Levels
- Modbus Application Protocol V1.1b3, Section 7 -- Exception Responses
