# Assembly Object (Class 0x04)

The Assembly Object binds attributes of multiple objects into a single block of data. It is the primary mechanism for I/O data exchange in EtherNet/IP.

## Instances

`goeip` supports dynamic registration of Assembly Instances. Common instance types include:

- **Input Assembly**: Data produced by the Target and consumed by the Originator (T->O).
- **Output Assembly**: Data produced by the Originator and consumed by the Target (O->T).
- **Configuration Assembly**: Configuration data sent to the Target during connection establishment.

## Services

| Service Code | Service Name | Description |
|--------------|--------------|-------------|
| `0x0E` | `Get_Attribute_Single` | Reads the data of an assembly instance (Attribute 3). |
| `0x10` | `Set_Attribute_Single` | Writes the data of an assembly instance (Attribute 3). |

## Usage

### Registering Assemblies

You can register assemblies with arbitrary IDs and data buffers:

```go
import "github.com/iceisfun/goeip/pkg/objects/assembly"

ao := assembly.NewAssemblyObject()

// Register Input Assembly (Instance 100)
inputData := make([]byte, 32)
ao.RegisterAssembly(100, inputData)

// Register Output Assembly (Instance 150)
outputData := make([]byte, 32)
ao.RegisterAssembly(150, outputData)
```

### Accessing Data

The data in an assembly can be accessed or modified thread-safely via the `GetAttributeSingle` and `SetAttributeSingle` methods, or by the internal Runtime engine during I/O exchange.

```go
// Read Data
data, err := ao.GetAttributeSingle(100, 3) // Attribute 3 is Data

// Write Data
newData := []byte{...}
err := ao.SetAttributeSingle(150, 3, newData)
```
