# Modbus TCP Server Example

This example demonstrates a full-featured Modbus TCP server built with the `goindustrial` library. It creates a server that accepts Modbus TCP client connections, serves data from an in-memory store, and reports live status information.

## What This Example Does

1. **Creates an in-memory data store** pre-populated with sample data across all four Modbus data areas (coils, discrete inputs, holding registers, input registers).
2. **Starts a Modbus TCP server** that listens for client connections on a configurable address and port.
3. **Registers lifecycle callbacks** that log when clients connect and disconnect.
4. **Prints periodic status reports** every 10 seconds showing connected clients, transaction counts, and per-function-code statistics.
5. **Handles OS signals** (SIGINT/SIGTERM) for graceful shutdown, ensuring all client connections are cleanly closed.

## How to Run

```bash
# Run with defaults (binds to 0.0.0.0:5020)
go run ./examples/modbus/server

# Run on a custom port
go run ./examples/modbus/server -port 5030

# Bind to loopback only
go run ./examples/modbus/server -addr 127.0.0.1 -port 5020
```

### Command-Line Flags

| Flag    | Default     | Description                                    |
|---------|-------------|------------------------------------------------|
| `-addr` | `0.0.0.0`  | Network interface address to bind to           |
| `-port` | `5020`     | TCP port to listen on                          |

> **Note:** The standard Modbus TCP port is 502, but binding to ports below 1024 requires root/administrator privileges on most operating systems. This example defaults to 5020 for convenience.

## Testing with Modbus Client Tools

Once the server is running, you can connect to it with any Modbus TCP client. Here are some options:

### Using the goindustrial client examples

```bash
# In another terminal, read holding registers
go run ./examples/modbus/read_registers -addr 127.0.0.1 -port 5020

# Read coils
go run ./examples/modbus/read_coils -addr 127.0.0.1 -port 5020

# Run the all_data_types example
go run ./examples/modbus/all_data_types -addr 127.0.0.1 -port 5020
```

### Using modpoll (command-line tool)

[modpoll](https://www.modbusdriver.com/modpoll.html) is a popular command-line Modbus master (client) tool:

```bash
# Read 10 holding registers starting at address 0
modpoll -m tcp -a 1 -r 1 -c 10 -t 4 127.0.0.1 -p 5020

# Read 10 coils starting at address 0
modpoll -m tcp -a 1 -r 1 -c 10 -t 0 127.0.0.1 -p 5020

# Write value 42 to holding register at address 0
modpoll -m tcp -a 1 -r 1 -t 4 127.0.0.1 -p 5020 42
```

### Using pymodbus (Python)

```python
from pymodbus.client import ModbusTcpClient

client = ModbusTcpClient('127.0.0.1', port=5020)
client.connect()

# Read 10 holding registers starting at address 0
result = client.read_holding_registers(0, 10)
print(result.registers)

# Write a single register
client.write_register(0, 2000)

client.close()
```

## Expected Output

```
[2026-03-26T10:00:00-05:00] INFO: Pre-populating data store with sample data
[2026-03-26T10:00:00-05:00] INFO: Starting Modbus TCP server on 0.0.0.0:5020
[2026-03-26T10:00:00-05:00] INFO: Modbus TCP server started on 0.0.0.0:5020
[2026-03-26T10:00:00-05:00] INFO: Server is running. Press Ctrl+C to stop.
[2026-03-26T10:00:10-05:00] INFO: [STATUS] Server running | No clients connected
[2026-03-26T10:00:15-05:00] INFO: CLIENT CONNECTED: 127.0.0.1:54321
[2026-03-26T10:00:15-05:00] INFO: New client connected: 127.0.0.1:54321
[2026-03-26T10:00:20-05:00] INFO: [STATUS] Server running | 1 client(s) connected:
[2026-03-26T10:00:20-05:00] INFO: [STATUS]   Client 1: 127.0.0.1:54321 | connected 5s | rx: 12 tx: 12 | fc: ReadCoils=3 ReadHoldingRegisters=5 WriteSingleRegister=4
[2026-03-26T10:00:22-05:00] INFO: CLIENT DISCONNECTED: 127.0.0.1:54321 (rx=15, tx=15)
^C
[2026-03-26T10:00:30-05:00] INFO: Received signal: interrupt. Shutting down...
[2026-03-26T10:00:30-05:00] INFO: Modbus TCP server stopped

--- Final Data Store State ---
Memory Store Content:
Coils:
  0: true
  1: false
  ...
Holding Registers:
  0: 2000 (0x07D0)
  ...
[2026-03-26T10:00:30-05:00] INFO: Server stopped cleanly. Goodbye.
```

## Modbus Server Concepts

### The Data Store

A Modbus server (historically called a "slave") maintains four data tables, as defined in the Modbus Application Protocol Specification V1.1b3, Section 4.3:

| Data Area           | Function Codes | Type     | Access     | Address Range |
|---------------------|---------------|----------|------------|---------------|
| Coils               | FC 01, 05, 0F | Boolean  | Read/Write | 0 - 65535     |
| Discrete Inputs     | FC 02          | Boolean  | Read-Only  | 0 - 65535     |
| Holding Registers   | FC 03, 06, 10 | uint16   | Read/Write | 0 - 65535     |
| Input Registers     | FC 04          | uint16   | Read-Only  | 0 - 65535     |

"Read-Only" means that the Modbus protocol does not define write function codes for those areas. The server application itself is free to update discrete inputs and input registers (for example, from sensor readings). The `MemoryStore` exposes `SetDiscreteInput()` and `SetInputRegister()` methods for this purpose.

### Default Handlers

When you call `modbus.NewServer()`, the server automatically registers handlers for all standard function codes. Each handler reads from or writes to the configured `DataStore`. You can override any handler with `server.SetHandler(fc, handlerFunc)` to implement custom logic such as:

- Input validation beyond what the protocol requires
- Side effects (triggering hardware actions when a coil is written)
- Access control per client or per address range
- Mapping virtual Modbus addresses to non-contiguous storage

### Client Lifecycle

The server tracks every connected client in memory. The `ConnectedClient` snapshot includes:

- **RemoteAddr**: The client's IP:port string
- **ConnectedAt**: When the connection was established
- **RxTransactions / TxTransactions**: Number of complete request/response cycles
- **FunctionCodeStats**: Per-function-code request counts (useful for auditing which operations are most frequent)

### Graceful Shutdown

Calling `server.Stop(ctx)` performs an orderly shutdown:

1. Closes the TCP listener (no new connections accepted)
2. Closes all active client connections
3. Fires `OnClientDisconnect` for each connected client
4. Marks the server as not running

This ensures that clients receive a TCP RST or FIN rather than timing out, and that your application can log final statistics.

## Modbus Specification References

- **Modbus Application Protocol Specification V1.1b3** (Modbus Organization, 2012)
  - Section 4.1: MBAP Header (TCP framing)
  - Section 4.3: Data Model (the four data tables)
  - Section 6: Function Code Descriptions
  - Section 7: Exception Responses
- **Modbus Messaging on TCP/IP Implementation Guide V1.0b** (Modbus Organization, 2006)
  - Describes the TCP transport layer, port 502, connection management

## Architecture Notes

- The server runs an **accept loop** in a dedicated goroutine. Each accepted connection spawns its own goroutine for reading MBAP-framed requests.
- The `MemoryStore` is protected by an `sync.RWMutex`, making it safe for concurrent access from multiple client handler goroutines.
- Transaction counters use `atomic.Uint64` for lock-free, goroutine-safe incrementing.
- The server's `ConnectedClients()` method returns a **snapshot** (copies, not references), so it is safe to iterate and log without holding server locks.
