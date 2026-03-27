# Read/Write Multiple Registers Example

Demonstrates the Read/Write Multiple Registers function (FC 0x17), which performs an atomic write and read in a single Modbus transaction.

## What It Does

This example connects to a Modbus TCP server and performs the following:

1. Reads the current state of both the read and write target registers (for comparison).
2. Executes a single FC 0x17 request that atomically writes values to one register range and reads values from another register range.
3. Displays the read results returned by the combined operation.
4. Performs a separate read-back to verify the written values.

## Modbus Concepts

### ReadWriteMultipleRegisters (FC 0x17)

Function code 0x17 combines a Write Multiple Registers and a Read Holding Registers operation into a single atomic transaction. This is defined in Section 6.17 of the Modbus specification.

Key characteristics:
- The **write is executed before the read** on the server side.
- If the read and write address ranges overlap, the read returns the newly written values.
- The response contains only the read data (the write is confirmed implicitly by the absence of an exception).
- Read limit: 125 registers. Write limit: 121 registers.

### When to Use FC 0x17

FC 0x17 is most useful when:
- You need to update a control parameter and immediately observe its effect on a status register.
- You are implementing a command/response handshake through registers.
- You want to minimize network round-trips in latency-sensitive applications.

### Atomicity

The combined operation is atomic from the Modbus protocol perspective: the server processes the entire request as a single unit. There is no window where another client could interleave a request between the write and read portions.

### Device Support

Not all Modbus devices support FC 0x17. If the device returns exception code 0x01 (Function Code Not Supported), use separate FC 0x03 and FC 0x10 operations instead.

## How to Run

```bash
# Write [100,200,300] to address 0-2 and read 5 registers from address 0-4
go run ./examples/modbus/read_write_registers/ -addr 127.0.0.1 -port 502

# Write to one range, read from another
go run ./examples/modbus/read_write_registers/ \
  -addr 192.168.1.100 \
  -write-address 10 -write-values "500,600" \
  -read-address 0 -read-count 5

# Overlapping ranges: write to 0-2, then read back 0-4 (read sees written values)
go run ./examples/modbus/read_write_registers/ \
  -addr 127.0.0.1 -port 5020 \
  -write-address 0 -write-values "1111,2222,3333" \
  -read-address 0 -read-count 5

# Use with specific unit ID
go run ./examples/modbus/read_write_registers/ \
  -addr 192.168.1.100 -unit 2 \
  -write-address 100 -write-values "42" \
  -read-address 100 -read-count 1
```

### Flags

| Flag              | Default       | Description                                        |
|-------------------|---------------|----------------------------------------------------|
| `-addr`           | `127.0.0.1`   | Modbus TCP server address                          |
| `-port`           | `502`         | Modbus TCP port                                    |
| `-unit`           | `1`           | Modbus unit ID (slave address, 0-247)              |
| `-read-address`   | `0`           | Starting address for the read portion (0-65535)    |
| `-read-count`     | `5`           | Number of registers to read (1-125)                |
| `-write-address`  | `0`           | Starting address for the write portion (0-65535)   |
| `-write-values`   | `100,200,300` | Comma-separated register values to write           |

## Expected Output

```
Connecting to Modbus TCP server at 127.0.0.1:502 (unit ID 1)...
Connected successfully.

--- Read/Write Multiple Registers (FC 0x17) ---

This function performs an atomic write-then-read in a single transaction.
The write is executed BEFORE the read on the server side.

  Write: 3 register(s) starting at address 0
  Write values: 100, 200, 300
  Read:  5 register(s) starting at address 0

  Current state of write-target registers:
    Address 0 = 0 (0x0000)
    Address 1 = 0 (0x0000)
    Address 2 = 0 (0x0000)

  Current state of read-target registers:
    Address 0 = 0 (0x0000)
    Address 1 = 0 (0x0000)
    Address 2 = 0 (0x0000)
    Address 3 = 0 (0x0000)
    Address 4 = 0 (0x0000)

  Executing ReadWriteMultipleRegisters (FC 0x17)...
  Operation successful.

  Read results (from the FC 0x17 response):
  Address    Decimal    Hex
  -------    -------    ------
  0          100        0x0064
  1          200        0x00C8
  2          300        0x012C
  3          0          0x0000
  4          0          0x0000

  Verifying write by reading back written registers (FC 0x03):
  Address    Written      Read Back    Match
  -------    -------      ---------    -----
  0          100          100          OK
  1          200          200          OK
  2          300          300          OK

  Verification: PASS -- all written values confirmed.

Done.
```

Note how addresses 0-2 in the read results reflect the just-written values (100, 200, 300) because the write executes before the read.

## Common Errors and Troubleshooting

| Error | Cause | Fix |
|-------|-------|-----|
| `Modbus exception: Function Code Not Supported (0x01)` | Device does not implement FC 0x17 | Use separate FC 0x03 and FC 0x10 operations |
| `Modbus exception: Data Address Not Available (0x02)` | Read or write address range is invalid | Check both address ranges against the device register map |
| `Modbus exception: Invalid Data Value (0x03)` | Write values or quantities are out of range | Verify values are 0-65535 and quantities are within limits |
| `connection refused` | No server at the specified address | Verify server address and port |

## Specification References

- Modbus Application Protocol V1.1b3, Section 6.17 -- Read/Write Multiple Registers (FC 0x17)
- Modbus Application Protocol V1.1b3, Section 6.3 -- Read Holding Registers (FC 0x03)
- Modbus Application Protocol V1.1b3, Section 6.12 -- Write Multiple Registers (FC 0x10)
- Modbus Application Protocol V1.1b3, Section 7 -- Exception Responses
