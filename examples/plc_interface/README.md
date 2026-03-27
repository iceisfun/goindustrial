# Protocol-Agnostic PLC Interface Example

This example demonstrates how to write **protocol-agnostic** code using the `plc.PLC` interface. The same Read/Write logic works with both Modbus TCP and EtherNet/IP -- the protocol is chosen at runtime via a command-line flag.

## What This Example Does

1. Accepts a `-protocol` flag (`"modbus"` or `"ethernetip"`) to select the industrial protocol
2. Creates the appropriate protocol-specific client
3. Uses the `plc.PLC` interface for all subsequent operations (Read, Write, Connect, Disconnect)
4. Demonstrates that the same application code works with both protocols

## The plc.PLC Abstraction

The `plc` package defines a minimal interface that all protocol clients implement:

```go
// plc.PLC is the common interface for all industrial controllers.
type PLC interface {
    Reader                              // Read(ctx, ...DataPoint) ([]Value, error)
    Writer                              // Write(ctx, DataPoint, []byte) error
    Connect(ctx context.Context) error
    Disconnect(ctx context.Context) error
    IsConnected() bool
}

// plc.Reader can read data points from a controller.
type Reader interface {
    Read(ctx context.Context, points ...DataPoint) ([]Value, error)
}

// plc.Writer can write data points to a controller.
type Writer interface {
    Write(ctx context.Context, point DataPoint, data []byte) error
}
```

Both `modbus.Client` and `ethernetip.Client` implement `plc.PLC`, verified at compile time:

```go
var _ plc.PLC = (*modbus.Client)(nil)
var _ plc.PLC = (*ethernetip.Client)(nil)
```

### DataPoints: Protocol-Specific Addressing

While the PLC interface is protocol-agnostic, the **data points** you pass to Read/Write are protocol-specific:

| Protocol | DataPoint Type | Fields | Example |
|----------|---------------|--------|---------|
| Modbus | `modbus.HoldingRegister` | `Addr`, `Qty` | `HoldingRegister{Addr: 0, Qty: 1}` |
| Modbus | `modbus.Coil` | `Addr`, `Qty` | `Coil{Addr: 100, Qty: 8}` |
| Modbus | `modbus.InputRegister` | `Addr`, `Qty` | `InputRegister{Addr: 0, Qty: 10}` |
| Modbus | `modbus.DiscreteInput` | `Addr`, `Qty` | `DiscreteInput{Addr: 0, Qty: 16}` |
| EtherNet/IP | `ethernetip.Tag` | `Name`, `Elements` | `Tag{Name: "MyDINT", Elements: 1}` |

All of these implement `plc.DataPoint`:

```go
type DataPoint interface {
    String() string
}
```

### Value: Protocol-Agnostic Results

The `plc.Value` returned by Read is the same regardless of protocol:

```go
type Value struct {
    DataPoint DataPoint  // Which point this value corresponds to
    Raw       []byte     // Raw bytes from the controller
}
```

The `Raw` bytes are protocol-specific:
- **Modbus**: big-endian register values (2 bytes per register)
- **EtherNet/IP**: includes a 2-byte CIP type code prefix followed by little-endian value data

## When to Use plc.PLC vs Protocol-Specific APIs

### Use plc.PLC When:

- You need **multi-protocol support** (e.g., a SCADA system that talks to both Modbus and EtherNet/IP devices)
- You want to write **reusable libraries** that work with any protocol (like the monitor package)
- You need **dependency injection** for testing (mock the plc.PLC interface)
- Your application's **business logic** doesn't depend on protocol-specific features

### Use Protocol-Specific APIs When:

- You need **protocol-specific features** (e.g., Modbus device identification, EtherNet/IP ListTags)
- You need **typed read/write** (e.g., `ethernetip.Read[int32](client, ctx, "MyDINT")`)
- You need **advanced protocol features** (e.g., EtherNet/IP connected messaging, Modbus read/write multiple registers in one transaction)
- You want **stronger typing** on values (e.g., `ReadHoldingRegisters` returns `[]uint16`, not `[]byte`)

### The Layered Approach

In practice, most applications use a layered approach:

```
Configuration Layer     (protocol-specific)
   - Creates concrete clients
   - Defines protocol-specific data points
   - Handles protocol-specific options
        |
        v
Business Logic Layer    (protocol-agnostic)
   - Accepts plc.PLC interface
   - Reads/writes using plc.DataPoint
   - Implements application logic
        |
        v
Monitoring Layer        (protocol-agnostic)
   - Uses plc.Reader for polling
   - Change detection
   - Event emission
```

## Usage

### Modbus Mode

```bash
# Read and write a Modbus holding register
go run . -protocol modbus -addr 127.0.0.1 -port 502 -register 0

# With a custom unit ID
go run . -protocol modbus -addr 127.0.0.1 -port 5020 -unit 1 -register 100
```

### EtherNet/IP Mode

```bash
# Read and write an EtherNet/IP tag
go run . -protocol ethernetip -addr 127.0.0.1:44818 -tag MyDINT
```

### Command-Line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-protocol` | `modbus` | Protocol: `modbus` or `ethernetip` |
| `-addr` | `127.0.0.1` | Server address (for EIP, include port: `host:44818`) |
| `-port` | `502` | Modbus TCP port (Modbus only) |
| `-register` | `0` | Holding register address (Modbus only) |
| `-unit` | `1` | Modbus unit/slave ID (Modbus only) |
| `-tag` | `MyDINT` | Tag name (EtherNet/IP only) |

## Example Output

```
Creating Modbus TCP client for 127.0.0.1:502 (unit ID 1)...
Connected via Modbus TCP.

Connection status: connected=true

--- Step 1: Read current value ---
  Reading HoldingRegister(addr=0, qty=1)...
  HoldingRegister(addr=0, qty=1) = 0x0000 (2 bytes)
    -> as uint16 (big-endian):    0
    -> as uint16 (little-endian): 0

--- Step 2: Write new value (42) ---
  Writing 2 bytes to HoldingRegister(addr=0, qty=1)...
  Write successful.

--- Step 3: Read back to confirm write ---
  Reading HoldingRegister(addr=0, qty=1)...
  HoldingRegister(addr=0, qty=1) = 0x002A (2 bytes)
    -> as uint16 (big-endian):    42
    -> as uint16 (little-endian): 10752

--- Step 4: Batch read (same point twice for demo) ---
  Batch reading 1 point(s)...
  [0] HoldingRegister(addr=0, qty=1) = 0x002A (2 bytes)

Done.
Disconnecting...
```

## Benefits for Multi-Protocol Systems

Using `plc.PLC` enables several architectural benefits:

1. **Single codebase**: Write monitoring, logging, and alarm logic once
2. **Runtime configuration**: Choose protocols from config files, not code changes
3. **Testing**: Mock the `plc.PLC` interface in unit tests without real hardware
4. **Extensibility**: Adding a new protocol only requires implementing the interface
5. **Consistency**: All protocol clients behave the same way at the interface level

## Write Data Encoding

The `plc.Write()` method accepts raw bytes, and the encoding is protocol-specific:

### Modbus Write Data

Modbus uses **big-endian** encoding (network byte order). A single holding register is 2 bytes:

```go
data := make([]byte, 2)
binary.BigEndian.PutUint16(data, 42)  // Write value 42
device.Write(ctx, modbus.HoldingRegister{Addr: 0, Qty: 1}, data)
```

### EtherNet/IP Write Data

EtherNet/IP requires a **2-byte CIP type code prefix** followed by **little-endian** value data:

```go
data := make([]byte, 6)
binary.LittleEndian.PutUint16(data[0:2], uint16(cip.TypeDINT))  // Type: DINT
binary.LittleEndian.PutUint32(data[2:6], 42)                     // Value: 42
device.Write(ctx, ethernetip.Tag{Name: "MyDINT", Elements: 1}, data)
```

This asymmetry in write encoding is one of the trade-offs of the generic interface. In practice, you would typically wrap the encoding logic in helper functions specific to your application's data model.
