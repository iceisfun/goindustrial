# pccc_read — read SLC 500 / MicroLogix data-table addresses over EtherNet/IP

This example reads a single PCCC data-table address (`N7:0`, `F8:5`,
`B3:0/2`, `T4:0.ACC`, `S:1`, …) from a legacy Allen-Bradley controller
(SLC 5/03, SLC 5/04, SLC 5/05, MicroLogix family). PLC-5 word-range
commands are out of scope.

## Why a separate command?

A Logix controller exposes named tags via the CIP Symbol Object — the
`read_tag` example uses that path. An SLC/MicroLogix does **not**: there
is no Symbol Object, only data-table files addressed by file type, file
number, element, and sub-element.

To talk to those controllers we tunnel a PCCC command inside the CIP
`Execute_PCCC` service (class `0x67`, service `0x4B`). The TCP / EIP /
CIP framing is identical to a Logix `Read Tag` exchange; only the inner
payload changes.

## Usage

```bash
# Read integer file 7, element 0:
go run . -addr 10.30.40.71:44818 -tag N7:0

# Read float file 8, element 5:
go run . -addr 10.30.40.71:44818 -tag F8:5

# Read bit 2 of bit-file 3, element 0:
go run . -addr 10.30.40.71:44818 -tag B3:0/2

# Read a timer accumulator:
go run . -addr 10.30.40.71:44818 -tag T4:0.ACC

# Read the whole timer element (control word + PRE + ACC):
go run . -addr 10.30.40.71:44818 -tag T4:0

# Read the status file:
go run . -addr 10.30.40.71:44818 -tag S:1
```

The default port is 44818 (EtherNet/IP). Use `-addr host` to omit it if
your `ethernetip.Connect` invocation supplies a default.

## Address syntax

| Form              | Meaning                                  | Sub/Bit            |
|-------------------|------------------------------------------|--------------------|
| `N<f>:<e>`        | integer file `f`, element `e`            | —                  |
| `F<f>:<e>`        | float file `f`, element `e` (4 bytes)    | —                  |
| `B<f>:<e>[/b]`    | bit file, optional bit `b` of word       | bit `b` in [0,15]  |
| `S:<e>`           | status file (always file 2)              | —                  |
| `I:<s>` / `O:<s>` | input / output image word                | implicit file 1 / 0|
| `T<f>:<e>.ACC`    | timer accumulator                        | sub-element 2      |
| `T<f>:<e>.PRE`    | timer preset                             | sub-element 1      |
| `T<f>:<e>.EN`     | timer enable bit (bit 15 of control)     | bit-in-control     |
| `C<f>:<e>.CU`     | counter up-enable bit (bit 15)           | bit-in-control     |
| `R<f>:<e>.LEN`    | control length (sub-element 1)           | —                  |
| `ST<f>:<e>`       | string file                              | —                  |

The full grammar — including all timer/counter/control field names — is
documented in the `pccc` package.

## What the code does

1. **Parse the address** with `pccc.ParseAddress`.
2. **Connect** with `ethernetip.Connect` (TCP dial + `RegisterSession`).
3. **Encode** a PCCC `PROTECTED TYPED LOGICAL READ` (FNC `0xA2`) with
   `pccc.EncodeTypedRead`.
4. **Send** via `client.ExecutePCCC` — this wraps the PCCC bytes in a CIP
   `Execute_PCCC` (`0x4B`) request and unwraps the requestor-ID echo
   from the reply.
5. **Decode** the reply with `pccc.DecodeReply`. A non-zero STS surfaces
   as `*pccc.Error` (not retried).
6. **Interpret** the data bytes based on the parsed file type — integer,
   float, bit, or a 3-word timer/counter/control element.

## Error handling

- **Transport errors** (connection drop) are retried by the client per
  `WithRetries` / `WithRetryDelay`.
- **CIP-level errors** (the controller's PCCC Object refuses the request)
  surface as `cip.Error`.
- **PCCC STS / EXT STS errors** surface as `*pccc.Error` after
  `pccc.DecodeReply`. Common codes:
  - `0x10` — illegal command or format
  - `0x70` — processor is in program mode
  - `0xF0`/`0x04` — symbol (address) not found
