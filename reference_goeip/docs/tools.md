# Tools & Usage

`goeip` includes two command-line tools to demonstrate and verify the functionality of the library.

![Barn Owl understanding tools](plcowl_cli.jpg)

## Adapter (Server)

The `adapter` tool simulates an EtherNet/IP target device. It listens for incoming connections and supports configurable Input and Output assemblies.

### Command Line Arguments

- `--addr`: TCP address to listen on (default `:44818`).
- `--udp-addr`: UDP address to listen on for I/O (default `:2222`).
- `--input-assembly`: ID of the Input Assembly (e.g., `100`). Can optionally specify a file to load data from (e.g., `100=data.bin`).
- `--output-assembly`: ID of the Output Assembly (e.g., `150`).

### Example

```bash
# Start Adapter with Input Instance 100 and Output Instance 150
./adapter --input-assembly 100 --output-assembly 150
```

## Scanner (Client)

The `scanner` tool simulates an EtherNet/IP originator. It connects to a target, establishes an I/O connection using `Forward_Open`, and exchanges cyclic data.

### Command Line Arguments

- `--addr`: Target TCP address (default `127.0.0.1:44818`).
- `--input-assembly`: Target's Input Assembly ID (T->O) (default `100`).
- `--output-assembly`: Target's Output Assembly ID (O->T) (default `150`).
- `--rpi`: Requested Packet Interval (default `100ms`).

### Example

```bash
# Connect to local adapter with 20ms RPI
./scanner --addr 127.0.0.1:44818 --rpi 20ms
```

## Verification Flow

1. **Start the Adapter**:
   ```bash
   ./adapter
   ```
   You should see logs indicating the server is listening.

2. **Start the Scanner**:
   ```bash
   ./scanner
   ```
   You should see:
   - "Forward_Open Successful!"
   - Logs indicating cyclic packet transmission.

3. **Observe Traffic**:
   You can use Wireshark to capture traffic on port 2222 (UDP) and 44818 (TCP) to verify the packet formats and timing.

## Explicit Messaging Tools

In addition to the implicit I/O tools, `goeip` provides tools for explicit messaging and discovery.

### list_identity

Sends a `ListIdentity` command to the target to retrieve device information (Vendor, Product Type, Product Code, Version, Status, Serial Number, Product Name).

```bash
go run ./cmd/list_identity -addr 192.168.1.10
```

### list_tags

Enumerates all tags on a Rockwell Logix controller using the Symbol Object (Class 0x6B). It retrieves the name and type of each tag.

```bash
go run ./cmd/list_tags -addr 192.168.1.10
```

### read_tag_single

Reads the value of a specific tag from the controller. Supports reading arrays by specifying an element count.

**Arguments:**

- `--addr`: Target Address (default `192.168.1.10:44818`)
- `--tag`: Tag Name to read.
- `--count`: Number of elements to read (default `1`, use higher values for arrays).

```bash
# Read a single tag
go run ./cmd/read_tag_single -addr 192.168.1.10 -tag MyTag

# Read a DINT[21] array
go run ./cmd/read_tag_single -addr 192.168.1.10 -tag MyDINTArray -count 21
```

### read_tag_single_reconnecting

Demonstrates the `ReconnectingClient` which automatically handles disconnections and reconnections. It reads a tag repeatedly and survives network interruptions.

```bash
go run ./cmd/read_tag_single_reconnecting -addr 192.168.1.10 -tag MyTag
```

### read_tag_struct

Demonstrates reading tags directly into Go types. Supported types include `bool`, `int8`, `uint8`, `int16`, `uint16`, `int32`, `uint32`, `int64`, `uint64`, `float32`, `float64`, `timer`, and `custom` (user-defined struct).

```bash
go run ./cmd/read_tag_struct -addr 192.168.1.10 -tag MyTag -type int32
```

### read_tag_timer

Reads a specific Rockwell Logix Timer structure (PRE, ACC, EN, TT, DN) from the controller.

```bash
go run ./cmd/read_tag_timer -addr 192.168.1.10 -tag MyTimer
```

### write_tag_single

Writes a value to a specific tag on the controller. You must specify the tag name, the data type, and the value.

**Arguments:**

- `--addr`: Target Address (default `192.168.1.10:44818`)
- `--tag`: Tag Name to write.
- `--type`: Data Type of the tag (`BOOL`, `SINT`, `INT`, `DINT`, `LINT`, `USINT`, `UINT`, `UDINT`, `ULINT`, `REAL`, `LREAL`, `STRING`).
- `--value`: Value to write.

**Example:**

```bash
# Write 12345 to DINT tag 'MyTag'
go run ./cmd/write_tag_single -addr 192.168.1.10 -tag MyTag -type DINT -value 12345

# Write 3.14 to REAL tag 'MyFloat'
go run ./cmd/write_tag_single -addr 192.168.1.10 -tag MyFloat -type REAL -value 3.14

# Write "Hello" to STRING tag 'MyString'
go run ./cmd/write_tag_single -addr 192.168.1.10 -tag MyString -type STRING -value "Hello"
```
