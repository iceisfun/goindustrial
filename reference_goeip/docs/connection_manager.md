# CIP Connection Manager (Class 0x06)

The Connection Manager Object is responsible for establishing and managing logical connections between CIP devices. `goeip` implements a robust Connection Manager capable of handling both standard and large connections.

## Supported Services

| Service Code | Service Name | Description |
|--------------|--------------|-------------|
| `0x54` | `Forward_Open` | Opens a standard connection (up to 511 bytes). |
| `0x5B` | `Large_Forward_Open` | Opens a large connection (up to 65535 bytes). |
| `0x4E` | `Forward_Close` | Closes an existing connection. |

## Implementation Details

The implementation is located in `pkg/objects/connmgr`.

### Connection Allocation

When a `Forward_Open` request is received, the Connection Manager:
1. Parses the request parameters (RPI, Connection Parameters, Path).
2. Allocates a unique **Target-to-Originator (T->O) Connection ID**.
3. Stores the connection state, mapping the Triad (Connection Serial, Vendor ID, Originator Serial) to the connection.
4. Returns a success response containing the allocated T->O ID and the actual RPIs.

### Large Connections

The `Large_Forward_Open` service is supported for applications requiring data payloads larger than the standard 511-byte limit. It uses 32-bit fields for Network Connection Parameters, allowing for much larger packet sizes.

### Usage

To use the Connection Manager in your application, instantiate it and register it with the Message Router:

```go
import (
    "github.com/iceisfun/goeip/pkg/cip"
    "github.com/iceisfun/goeip/pkg/objects/connmgr"
)

// Create Connection Manager
cm := connmgr.NewConnectionManager()

// Register with Router
router := cip.NewMessageRouter()
router.RegisterObject(cip.ClassConnectionMgr, cm)
```
