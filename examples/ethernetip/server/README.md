# EtherNet/IP Server Example

This example demonstrates how to build an **EtherNet/IP server** (also called an "adapter" in CIP terminology) using the `goindustrial` library. The server accepts incoming EtherNet/IP TCP connections and routes CIP (Common Industrial Protocol) requests to registered objects.

## What This Example Does

1. Creates a **CIP Message Router** -- the central dispatcher for all CIP requests
2. Implements a **custom CIP object** (`TagObject`) that stores tag values in memory
3. Pre-populates the tag database with demo tags (`MyDINT`, `MyREAL`, `MySTRING`, `Counter`)
4. Starts an **EtherNet/IP TCP server** on the specified address
5. Handles **graceful shutdown** via OS signals (Ctrl+C or SIGTERM)

## EtherNet/IP Server Architecture

An EtherNet/IP server processes requests in layers:

```
EtherNet/IP Client (TCP connection)
        |
        v
ethernetip.Server
   - Accepts TCP connections
   - Handles EIP session registration/unregistration
   - Parses SendRRData (unconnected messaging) and SendUnitData (connected messaging)
   - Extracts the CIP Message Router Request from the EIP encapsulation
        |
        v
cip.MessageRouter
   - Parses the Request Path to find the destination Class ID
   - Looks up the registered cip.Object for that Class ID
   - Passes the remaining path + request data to the object
        |
        v
cip.Object (your custom implementation)
   - Handles the service request (ReadTag, WriteTag, GetAttributeSingle, etc.)
   - Returns response data or a CIP error
```

### CIP Message Routing

The CIP protocol organizes functionality into **objects**, each identified by a **Class ID**. When a client sends a request, the path in the request specifies which class and instance to target. The Message Router reads the Class ID from the first segment of the path and dispatches the request to the corresponding registered object.

For example, a ReadTag request for tag "MyDINT" might have a path like:

```
[0x20] [0x04]  -- Class segment: Class 0x04 (Assembly)
[0x91] [0x06] [M] [y] [D] [I] [N] [T]  -- Symbolic segment: "MyDINT"
```

The router extracts Class ID 0x04, finds our `TagObject`, and calls its `HandleRequest` method with the remaining path (the symbolic segment).

### Implementing a CIP Object

To create a custom CIP object, implement the `cip.Object` interface:

```go
type Object interface {
    HandleRequest(service cip.USINT, path cip.Path, data []byte) ([]byte, error)
}
```

- **service**: The CIP service code (e.g., `0x4C` for ReadTag, `0x4D` for WriteTag)
- **path**: The remaining EPATH after the Message Router consumed the Class segment
- **data**: The service-specific request payload
- **Return**: Response data bytes on success, or a `cip.Error` on failure

The `cip.Error` type lets you return standard CIP status codes:

```go
return nil, cip.Error{Status: cip.StatusPathDestinationUnknown}  // tag not found
return nil, cip.Error{Status: cip.StatusServiceNotSupported}      // unknown service code
```

### Supported CIP Services

This example's `TagObject` supports:

| Service | Code | Description |
|---------|------|-------------|
| ReadTag | `0x4C` | Read a tag value by symbolic name |
| WriteTag | `0x4D` | Write a tag value by symbolic name |
| GetAttributeAll | `0x01` | Return a summary of the object (tag count) |

### Tag Data Types

Tags are stored with their CIP data type code. This example pre-populates:

| Tag Name | CIP Type | Value |
|----------|----------|-------|
| `MyDINT` | DINT (0x00C4) | 12345 |
| `MyREAL` | REAL (0x00CA) | 3.14 |
| `MySTRING` | STRING (0x00D0) | "Hello, EIP!" |
| `Counter` | DINT (0x00C4) | 0 |

## Usage

```bash
# Start the server on the default EtherNet/IP port
go run . -addr :44818

# Start on a custom port (useful for non-root testing)
go run . -addr :44818
```

### Command-Line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `:44818` | TCP address to listen on (`host:port`) |

### Testing with Client Examples

Once the server is running, you can test it with other examples in this repository:

```bash
# Read a pre-populated tag
go run ../read_tag -addr 127.0.0.1:44818 -tag MyDINT

# Write a new value
go run ../write_tag -addr 127.0.0.1:44818 -tag MyDINT -value 42

# Read the updated value
go run ../read_tag -addr 127.0.0.1:44818 -tag MyDINT
```

## Graceful Shutdown

The server listens for `SIGINT` (Ctrl+C) and `SIGTERM` signals. When received:

1. `srv.Stop()` is called
2. The TCP listener is closed (no new connections accepted)
3. Existing client connections are terminated
4. The process exits cleanly

This pattern is important in production environments where the server may be managed by systemd, Docker, or Kubernetes, which send SIGTERM before forcibly killing the process.

## Extending This Example

To build a more complete EtherNet/IP device, you could:

- **Add an Identity Object** (Class 0x01) to respond to ListIdentity requests
- **Add a Connection Manager** (Class 0x06) for connected messaging (implicit I/O)
- **Persist tag values** to disk or a database instead of in-memory storage
- **Add access control** to restrict which clients can write to tags
- **Implement array tags** by handling the element count in ReadTag/WriteTag
- **Add data validation** to enforce type-safe writes (reject writing a REAL to a DINT tag)
