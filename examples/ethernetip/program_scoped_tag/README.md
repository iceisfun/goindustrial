# Program-Scoped Tag Example

Read a Rockwell Logix tag that lives inside a Program (not the controller
scope), including individual struct members.

## What This Example Does

Connects to a Logix controller and reads:

1. The whole COUNTER struct at `Program:MainProgram.MyCounter`
2. (Optionally) the `.ACC` member of that counter as a typed `int32`

## Why This Example Exists

Logix tag strings carry structural information beyond a simple name:

| Part        | Meaning                                                  |
| ----------- | -------------------------------------------------------- |
| `Program:X` | Tag is defined inside the `X` program, not at controller scope |
| `.Member`   | Access a member of a struct/UDT                          |
| `[5]`       | Index into an array (`[i,j,k]` for 2D/3D)                |
| `.5`        | Bit-level access into a BOOL within an integer           |

On the wire, the controller expects each `.`-separated piece to be a separate
EPATH segment -- it does **not** accept the whole dotted string as a single
ANSI Extended Symbol segment. The library handles the split internally via
`cip.ParseTagPath`, so calling code just passes the natural Logix tag string.

For example, `Program:MainProgram.Tote_Count_CNTR.ACC` becomes three
symbolic segments:

```
91 13 "Program:MainProgram" 00     -- symbol seg, 19 bytes + pad
91 0F "Tote_Count_CNTR"     00     -- symbol seg, 15 bytes + pad
91 03 "ACC"                 00     -- symbol seg,  3 bytes + pad
```

The colon stays inside the first symbol because it is part of the program
scope prefix, not a separator.

## How to Run

```bash
# Read the whole COUNTER struct
go run ./examples/ethernetip/program_scoped_tag \
  -addr 192.168.1.10:44818 \
  -tag Program:MainProgram.MyCounter

# Read the whole struct AND the .ACC member individually
go run ./examples/ethernetip/program_scoped_tag \
  -addr 192.168.1.10:44818 \
  -tag Program:MainProgram.MyCounter \
  -member ACC
```

## Expected Output

```
Connected to 192.168.1.10:44818

--- Whole struct: Program:MainProgram.MyCounter ---
PRE=100  ACC=42
CU=true  CD=false  DN=false  OV=false  UN=false

--- Member read: Program:MainProgram.MyCounter.ACC ---
ACC = 42
```

## Tag Path Syntax Cheatsheet

All tag-level APIs (`ReadTag`, `ReadCounter`, `ReadTimer`, `WriteTag`,
`Read[T]`, `ReadSlice[T]`, `ReadTagInto`, ...) accept these forms:

| String                              | EPATH                                            |
| ----------------------------------- | ------------------------------------------------ |
| `MyTag`                             | symbol("MyTag")                                  |
| `MyStruct.Field`                    | symbol("MyStruct") + symbol("Field")             |
| `MyArray[5]`                        | symbol("MyArray") + member(5)                    |
| `Matrix[2,3]`                       | symbol("Matrix") + member(2) + member(3)         |
| `Program:MainProgram.MyTag`         | symbol("Program:MainProgram") + symbol("MyTag")  |
| `Program:Foo.Bar[5].Baz`            | 3 symbols + member(5)                            |
| `Local:2:I.Data[0]`                 | symbol("Local:2:I") + symbol("Data") + member(0) |
| `MyDINT.5`                          | symbol("MyDINT") + member(5)                     |

## Common Errors

| Error                                | Cause                                           |
| ------------------------------------ | ----------------------------------------------- |
| `CIP error: status=0x04`             | Path Segment Error -- usually a malformed tag string, e.g. a stray `:` between the program scope and the tag name (use `.` instead) |
| `CIP error: status=0x05`             | Path Destination Unknown -- tag does not exist on the controller, or the program name is wrong |
| `insufficient data for Counter: ...` | Tag is not a COUNTER struct -- use `ReadTag` for atomic types |
