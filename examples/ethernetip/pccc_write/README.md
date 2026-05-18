# pccc_write — write a single PCCC value

Quick CLI for writing one SLC data-table value. The Go type used by the
write is inferred from the address: float files take `-value 3.14`, bit
suffixes (`B3:0/2`, `T4:0.EN`) take `-value 0` or `-value 1` and use a
read-modify-write cycle, everything else takes an integer.

## Usage

```bash
# Integer
go run . -addr 10.30.40.71:44818 -tag N7:0 -value 42

# Float
go run . -addr 10.30.40.71:44818 -tag F8:5 -value 3.14

# Bit (read-modify-write)
go run . -addr 10.30.40.71:44818 -tag B3:0/2 -value 1
```
