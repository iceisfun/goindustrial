# EtherNet/IP Device Probe

Queries an EtherNet/IP device over CIP and reports everything it can discover: device identity, network configuration, Ethernet link status, assembly instances, supported CIP object classes, connection manager statistics, and optionally Logix controller tags with live values.

## What This Example Does

1. Connects to the device via TCP (port 44818) and registers an EIP session
2. Reads the **Identity Object** (Class 0x01) for product name, vendor, device type, revision, serial number
3. Reads the **TCP/IP Interface** (Class 0xF5) for IP configuration, hostname, and capability flags
4. Reads the **Ethernet Link** (Class 0xF6) for interface speed, duplex, link status, and MAC address
5. Scans **Assembly instances** (Class 0x04) from 1 to 255 (configurable), reporting data size and contents
6. Reads **Connection Manager** (Class 0x06) statistics: open/close/reject counts
7. Probes known **CIP Object Classes** to discover which ones the device supports
8. Optionally lists **Logix tags** (Class 0x6B Symbol Object) and reads sample scalar values

## Running the Example

Basic probe:

```bash
go run ./examples/ethernetip/probe/ 192.168.1.10
```

Include Logix tag listing and sample values:

```bash
go run ./examples/ethernetip/probe/ 192.168.1.10 -tags
```

Scan more assembly instances (up to 500):

```bash
go run ./examples/ethernetip/probe/ 192.168.1.10 -assemblies=500
```

Increase timeout for slow networks:

```bash
go run ./examples/ethernetip/probe/ 192.168.1.10 -timeout=10s
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-assemblies` | `255` | Maximum assembly instance number to scan |
| `-tags` | `false` | List Logix controller tags and read sample values |
| `-timeout` | `5s` | Connection and request timeout |

## Expected Output

### AC/DC Drive (PowerFlex 525)

```
═══════════════════════════════════════════════════════════════
  DEVICE IDENTITY
═══════════════════════════════════════════════════════════════
  Product Name:    PowerFlex 525 3P 230V    .50HP
  Vendor ID:       1 (Rockwell Automation)
  Device Type:     150 (AC/DC Drive (vendor specific))
  Product Code:    9
  Revision:        7.1
  Serial Number:   0x706744FD
  Status:          0x0061

═══════════════════════════════════════════════════════════════
  ETHERNET LINK
═══════════════════════════════════════════════════════════════
  Interface Speed: 100 Mbps
  Link Status:     active, full duplex
  MAC Address:     5C:21:67:46:1C:1A

═══════════════════════════════════════════════════════════════
  ASSEMBLY INSTANCES
═══════════════════════════════════════════════════════════════
  Instance   1:   12 bytes  [0D 06 00 00 ...]  (word0=0x060D word1=0x0000)
  Instance   2:   12 bytes  [00 00 10 0E ...]  (word0=0x0000 word1=0x0E10)
  Total: 2 instances
```

### Servo Drive (Teknic ClearLink)

```
═══════════════════════════════════════════════════════════════
  DEVICE IDENTITY
═══════════════════════════════════════════════════════════════
  Product Name:    ClearLink
  Vendor ID:       424 (Teknic)
  Device Type:     43 (Servo Drive)

═══════════════════════════════════════════════════════════════
  ASSEMBLY INSTANCES
═══════════════════════════════════════════════════════════════
  Instance 100:  332 bytes  [00 00 00 00 00 00 00 00 ...]
  Instance 101:  228 bytes  [01 00 01 00 00 00 00 00 ...]
  Instance 112:  280 bytes  [00 00 00 00 00 00 00 00 ...]
  Instance 113:  200 bytes  [00 00 01 00 00 00 00 00 ...]
  Instance 150:  232 bytes  [00 00 00 00 00 00 00 00 ...]
  Instance 151:  120 bytes  [64 64 64 64 64 00 00 00 ...]
  Total: 6 instances
```

### Logix Controller (CompactLogix 5380)

```
═══════════════════════════════════════════════════════════════
  DEVICE IDENTITY
═══════════════════════════════════════════════════════════════
  Product Name:    5069-L310ER/A
  Device Type:     14 (Programmable Logic Controller)

═══════════════════════════════════════════════════════════════
  ASSEMBLY INSTANCES
═══════════════════════════════════════════════════════════════
  (none found — device may need I/O module configured)

═══════════════════════════════════════════════════════════════
  CIP OBJECT CLASSES
═══════════════════════════════════════════════════════════════
  0x01  Identity                      (8 bytes)
  0x02  Message Router                (134 bytes)
  0x06  Connection Manager            (8 bytes)
  0x6B  Symbol                        (41 bytes)
  0xAC  IO (Logix)                    (32 bytes)
  0xF5  TCP/IP Interface              (30 bytes)
  0xF6  Ethernet Link                 (36 bytes)
```

## Using the Probe for I/O Connection Setup

The assembly instance listing is key for setting up implicit I/O connections. When you see assemblies like:

```
  Instance   1:   12 bytes  (T→O input — drive status)
  Instance   2:   12 bytes  (O→T output — drive command)
```

You can use these as the connection points for the `io_scanner` example:

```bash
go run ./examples/ethernetip/io_scanner/ \
  -addr <IP> \
  -to-instance 1 -to-size 12 \
  -ot-instance 2 -ot-size 12
```

For devices with multiple assembly profiles (like the ClearLink with 6 instances), the probe helps identify which pairs are valid connection points and what sizes they expect.

## What the Sections Mean

| Section | CIP Class | What it tells you |
|---------|-----------|-------------------|
| Device Identity | 0x01 | Product name, vendor, firmware version, serial number |
| TCP/IP Interface | 0xF5 | IP address, subnet, gateway, DHCP/BOOTP capability |
| Ethernet Link | 0xF6 | Link speed, duplex, MAC address |
| Assembly Instances | 0x04 | Available I/O buffers with sizes — needed for Forward_Open |
| Connection Manager | 0x06 | How many connections have been opened/rejected — shows I/O activity |
| CIP Object Classes | Various | Which protocol features the device supports |
| Tags (Logix only) | 0x6B | Controller variables that can be read/written via explicit messaging |
