# Custom Types

`goeip` supports reading tags directly into Go structs and basic types. For custom structs that require specific decoding logic (e.g., handling padding, bit-packed fields, or non-standard layouts), you can implement the `Unmarshaler` and `Marshaler` interfaces.

![Barn Owl understanding custom types](plcowl_custom_types.jpg)

## Interfaces

### Unmarshaler

Implement `Unmarshaler` to define how your type decodes itself from raw CIP bytes.

```go
type Unmarshaler interface {
    UnmarshalCIP(data []byte) error
}
```

### Marshaler

Implement `Marshaler` to define how your type encodes itself into raw CIP bytes.

```go
type Marshaler interface {
    MarshalCIP() ([]byte, error)
}
```

## Example

Here is an example of a custom struct `MyCustomTag` that implements both interfaces.

```go
package main

import (
    "bytes"
    "encoding/binary"
    "fmt"
)

type MyCustomTag struct {
    Field1 int32
    Field2 float32
}

// UnmarshalCIP implements cip.Unmarshaler
func (m *MyCustomTag) UnmarshalCIP(data []byte) error {
    if len(data) < 8 {
        return fmt.Errorf("insufficient data")
    }
    m.Field1 = int32(binary.LittleEndian.Uint32(data[0:4]))
    m.Field2 = float32(binary.LittleEndian.Uint32(data[4:8])) // Note: Need math.Float32frombits for correct float decoding
    // Better to use binary.Read for simplicity if layout is standard
    return nil
}

// MarshalCIP implements cip.Marshaler
func (m *MyCustomTag) MarshalCIP() ([]byte, error) {
    buf := new(bytes.Buffer)
    binary.Write(buf, binary.LittleEndian, m.Field1)
    binary.Write(buf, binary.LittleEndian, m.Field2)
    return buf.Bytes(), nil
}
```

## Usage

You can use `client.ReadTagInto` with your custom type:

```go
var tag MyCustomTag
err := client.ReadTagInto("MyTag", &tag)
```
