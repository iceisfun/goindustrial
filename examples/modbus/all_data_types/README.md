# All Modbus Data Types Example

This example provides a comprehensive demonstration of every Modbus data area and all standard read/write operations. It exercises all four data tables, single and multiple read/write operations, atomic read-write transactions, and exception status queries.

## What This Example Does

The program connects to a Modbus TCP server and runs through six demonstration sections:

1. **Coils** -- Read 10 coils, write a single coil, write multiple coils, verify writes
2. **Discrete Inputs** -- Read 8 discrete inputs
3. **Holding Registers** -- Read 10 registers, write a single register, write multiple registers, verify writes
4. **Input Registers** -- Read 10 input registers
5. **Read/Write Multiple Registers** -- Atomic read + write in a single transaction (FC 17)
6. **Read Exception Status** -- Query the server's 8-bit exception status (FC 07)

Each operation is shown with clear section headers and verbose output.

## How to Run

First, start a Modbus TCP server. The server example in this repository works well:

```bash
# Terminal 1: start the server with pre-populated data
go run ./examples/modbus/server -port 5020
```

Then run this example:

```bash
# Terminal 2: run all data type demonstrations
go run ./examples/modbus/all_data_types -addr 127.0.0.1 -port 5020

# Use a different unit ID (for gateway setups)
go run ./examples/modbus/all_data_types -addr 127.0.0.1 -port 5020 -unit 1
```

### Command-Line Flags

| Flag    | Default       | Description                          |
|---------|---------------|--------------------------------------|
| `-addr` | `127.0.0.1`  | Modbus TCP server address            |
| `-port` | `5020`       | Modbus TCP server port               |
| `-unit` | `0`          | Modbus unit ID (slave address)       |

## Expected Output

```
[2026-03-26T10:00:00-05:00] INFO: Connecting to Modbus TCP server at 127.0.0.1:5020 (unit 0)
[2026-03-26T10:00:00-05:00] INFO: Connected successfully. Running all data type demonstrations.

========================================================================
================= COILS (Boolean, Read/Write) ==========================
========================================================================

--- Read Coils (FC 01) ---
Reading 10 coils from address 0...
  Coil 0 = ON
  Coil 1 = OFF
  Coil 2 = ON
  Coil 3 = ON
  Coil 4 = OFF
  Coil 5 = ON
  Coil 6 = OFF
  Coil 7 = OFF
  Coil 8 = ON
  Coil 9 = OFF

--- Write Single Coil (FC 05) ---
Writing coil 5 = ON...
  Success: coil 5 is now ON

--- Write Multiple Coils (FC 0F) ---
Writing 8 coils starting at address 0: [ON, OFF, ON, OFF, ON, ON, OFF, ON]
  Success: 8 coils written

--- Verify Coil Writes (FC 01) ---
Reading back 10 coils from address 0 to verify writes...
  Coil 0 = ON
  Coil 1 = OFF
  Coil 2 = ON
  Coil 3 = OFF
  Coil 4 = ON
  Coil 5 = ON
  Coil 6 = OFF
  Coil 7 = ON
  Coil 8 = ON
  Coil 9 = OFF

========================================================================
============= DISCRETE INPUTS (Boolean, Read-Only) ====================
========================================================================

--- Read Discrete Inputs (FC 02) ---
Reading 8 discrete inputs from address 0...
  Discrete Input 0 = ON
  Discrete Input 1 = ON
  Discrete Input 2 = OFF
  Discrete Input 3 = ON
  Discrete Input 4 = OFF
  Discrete Input 5 = ON
  Discrete Input 6 = ON
  Discrete Input 7 = OFF

========================================================================
=========== HOLDING REGISTERS (uint16, Read/Write) ====================
========================================================================

--- Read Holding Registers (FC 03) ---
Reading 10 holding registers from address 0...
  Holding Register 0 = 1000 (0x03E8)
  Holding Register 1 = 500 (0x01F4)
  Holding Register 2 = 60 (0x003C)
  ...

--- Write Single Register (FC 06) ---
Writing holding register 0 = 42...
  Success: holding register 0 is now 42

--- Write Multiple Registers (FC 10) ---
Writing 5 holding registers starting at address 5: [100 200 300 400 500]
  Success: 5 holding registers written

...

========================================================================
========================== SUMMARY =====================================
========================================================================

All operations completed successfully.
```

## The Modbus Data Model

The Modbus protocol defines four independent data tables. Every Modbus-compliant device organizes its data into some or all of these areas.

### The Four Tables

```
+--------------------+-----------+--------+---------------------+----------+
| Data Area          | Data Type | Access | Function Codes      | Quantity |
+--------------------+-----------+--------+---------------------+----------+
| Coils              | Boolean   | R/W    | FC 01, 05, 0F       | 1-2000   |
| Discrete Inputs    | Boolean   | R      | FC 02               | 1-2000   |
| Holding Registers  | uint16    | R/W    | FC 03, 06, 10, 17   | 1-125    |
| Input Registers    | uint16    | R      | FC 04               | 1-125    |
+--------------------+-----------+--------+---------------------+----------+
```

### Coils (FC 01, 05, 0F)

Coils are single-bit boolean values. The name comes from the original relay coils in early PLCs. In modern systems, coils represent any discrete (on/off) output or internal flag.

- **Read Coils (FC 01)**: Read 1 to 2000 contiguous coils. The response packs 8 coils per byte, LSB first.
- **Write Single Coil (FC 05)**: Set one coil to ON (0xFF00) or OFF (0x0000).
- **Write Multiple Coils (FC 0F)**: Write 1 to 1968 contiguous coils in a single request.

### Discrete Inputs (FC 02)

Discrete inputs are single-bit boolean values that are read-only from the Modbus client. They represent physical digital inputs: sensors, switches, and status contacts.

- **Read Discrete Inputs (FC 02)**: Read 1 to 2000 contiguous discrete inputs. Same bit-packing as coils.

The Modbus specification does not define a write function code for discrete inputs. The server application updates them from field I/O or internal logic.

### Holding Registers (FC 03, 06, 10, 17)

Holding registers are 16-bit unsigned integer (uint16) values. They are the most versatile data area and are used for setpoints, configuration, command words, and general-purpose data exchange.

- **Read Holding Registers (FC 03)**: Read 1 to 125 contiguous registers.
- **Write Single Register (FC 06)**: Write one register.
- **Write Multiple Registers (FC 10)**: Write 1 to 123 contiguous registers.
- **Read/Write Multiple Registers (FC 17)**: Atomically read up to 125 and write up to 121 registers in one transaction.

Each register is 2 bytes, transmitted big-endian (most significant byte first). For data wider than 16 bits (floats, 32-bit integers, strings), applications typically use consecutive registers. The byte order of multi-register values is not standardized by Modbus and must be agreed upon between client and server.

### Input Registers (FC 04)

Input registers are 16-bit unsigned integer values that are read-only from the Modbus client. They typically hold measured process values: analog inputs, sensor readings, counters, and status words.

- **Read Input Registers (FC 04)**: Read 1 to 125 contiguous registers.

Like discrete inputs, there is no write function code. The server updates them from hardware or internal computation.

### Address Spaces

Each data area has its own independent address space ranging from 0 to 65535. Address 0 in the coil table is completely unrelated to address 0 in the holding register table.

Some legacy documentation uses Modbus "reference numbers" with prefixed ranges:

| Prefix  | Data Area          | Address Range   |
|---------|--------------------|-----------------|
| 0xxxxx  | Coils              | 00001 - 09999   |
| 1xxxxx  | Discrete Inputs    | 10001 - 19999   |
| 3xxxxx  | Input Registers    | 30001 - 39999   |
| 4xxxxx  | Holding Registers  | 40001 - 49999   |

However, the actual Modbus PDU always uses zero-based 16-bit addresses (0-65535). The prefix convention is a display layer convenience, not part of the wire protocol. The `goindustrial` library uses zero-based addresses throughout.

### Read-Only vs Read/Write

"Read-only" means the Modbus protocol does not define write function codes for that data area. It does not mean the values are immutable:

- The **server application** can and should update discrete inputs and input registers. For example, a temperature sensor reading would be written to an input register by the server's sensor-polling loop.
- The **client** can only read these areas via FC 02 (discrete inputs) or FC 04 (input registers).
- For coils and holding registers, the client can both read and write.

## Function Code Reference

| FC   | Name                          | Data Area          | Direction | Max Quantity |
|------|-------------------------------|--------------------|-----------|-------------|
| 0x01 | Read Coils                    | Coils              | Read      | 2000        |
| 0x02 | Read Discrete Inputs          | Discrete Inputs    | Read      | 2000        |
| 0x03 | Read Holding Registers        | Holding Registers  | Read      | 125         |
| 0x04 | Read Input Registers          | Input Registers    | Read      | 125         |
| 0x05 | Write Single Coil             | Coils              | Write     | 1           |
| 0x06 | Write Single Register         | Holding Registers  | Write     | 1           |
| 0x07 | Read Exception Status         | (special)          | Read      | N/A         |
| 0x0F | Write Multiple Coils          | Coils              | Write     | 1968        |
| 0x10 | Write Multiple Registers      | Holding Registers  | Write     | 123         |
| 0x17 | Read/Write Multiple Registers | Holding Registers  | R/W       | R:125 W:121 |

## Modbus Specification References

- **Modbus Application Protocol Specification V1.1b3** (Modbus Organization, 2012)
  - Section 4.3: MODBUS Data Model (the four data tables)
  - Section 4.4: MODBUS Addressing Model (zero-based vs. reference numbers)
  - Section 6.1-6.17: Individual Function Code Descriptions
  - Section 7: Exception Responses
- **Modbus Messaging on TCP/IP Implementation Guide V1.0b** (Modbus Organization, 2006)

## Architecture Notes

- The example uses `modbus.Connect()` which internally creates a `ReconnectingTransport`. For a manual transport setup example, see the `reconnecting` example.
- Each operation uses its own `context.WithTimeout` to prevent hangs. The 5-second timeout is generous for local TCP but appropriate for production use over WANs.
- The `CoilValue` and `DiscreteInputValue` types are both aliases for `bool`. The `RegisterValue` and `InputRegisterValue` types are both aliases for `uint16`. These aliases exist for documentation clarity and type safety at the API level.
- Bit packing for coils follows the Modbus specification: LSB of the first byte corresponds to the lowest address. The client library handles all packing/unpacking transparently.
