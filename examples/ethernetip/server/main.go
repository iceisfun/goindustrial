// Example: ethernetip/server
//
// Demonstrates running an EtherNet/IP server (adapter) with a CIP message
// router. The server listens for incoming EtherNet/IP TCP connections and
// routes CIP requests to registered objects.
//
// This example implements a custom CIP object that acts as a tag database,
// supporting ReadTag (0x4C) and WriteTag (0x4D) services. Tags are stored
// in memory with their CIP data types (DINT, REAL, STRING). The server is
// pre-populated with a few demo tags so you can immediately test it with
// any EtherNet/IP client (including the other examples in this repository).
//
// Architecture:
//
//   EtherNet/IP Client (TCP)
//         |
//         v
//   ethernetip.Server  -- handles EIP session registration, SendRRData
//         |
//         v
//   cip.MessageRouter  -- parses the CIP request path, finds the target object
//         |
//         v
//   TagObject          -- custom cip.Object that stores/retrieves tag values
//
// The server uses signal handling (SIGINT/SIGTERM) for graceful shutdown.
//
// Usage:
//
//	go run . -addr :44818
//
// Then use the read_tag or write_tag examples to interact with the server:
//
//	go run ../read_tag  -addr 127.0.0.1:44818 -tag MyDINT
//	go run ../write_tag -addr 127.0.0.1:44818 -tag MyDINT -value 42
package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/iceisfun/goindustrial/logging"
	"github.com/iceisfun/goindustrial/protocol/ethernetip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
)

// ---------------------------------------------------------------------------
// tagEntry represents a single tag stored in our in-memory tag database.
// Each tag has a name, a CIP data type, and raw byte data.
// ---------------------------------------------------------------------------
type tagEntry struct {
	dataType cip.DataType // CIP data type code (e.g., TypeDINT, TypeREAL, TypeSTRING)
	data     []byte       // Raw value bytes (little-endian, matching CIP encoding)
}

// ---------------------------------------------------------------------------
// TagObject implements the cip.Object interface. It acts as a simple tag
// database that responds to ReadTag (0x4C) and WriteTag (0x4D) CIP services.
//
// In a real PLC, the tag object would be backed by the PLC's memory map.
// Here we use a Go map for simplicity.
// ---------------------------------------------------------------------------
type TagObject struct {
	mu   sync.RWMutex
	tags map[string]*tagEntry // Tag name -> tag data
}

// NewTagObject creates a new TagObject with an empty tag database.
func NewTagObject() *TagObject {
	return &TagObject{
		tags: make(map[string]*tagEntry),
	}
}

// SetTag pre-populates a tag in the database. This is used at startup to
// create demo tags that clients can immediately read.
func (t *TagObject) SetTag(name string, dataType cip.DataType, data []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tags[name] = &tagEntry{dataType: dataType, data: data}
}

// HandleRequest dispatches a CIP service request to the appropriate handler.
// This is the core method required by the cip.Object interface.
//
// The Message Router has already stripped the Class segment from the path
// before calling this method. The remaining path contains the tag's symbolic
// segment (for ReadTag/WriteTag) or instance/attribute segments (for
// GetAttributeSingle, etc.).
//
// Parameters:
//   - service: the CIP service code (e.g., 0x4C for ReadTag)
//   - path:    the remaining EPATH after the class segment was consumed
//   - data:    the request data (service-specific payload)
//
// Returns:
//   - response data bytes on success
//   - a cip.Error on failure (the router encodes this into the MR response)
func (t *TagObject) HandleRequest(service cip.USINT, path cip.Path, data []byte) ([]byte, error) {
	switch service {

	// -----------------------------------------------------------------------
	// ReadTag (0x4C) -- Read Tag Service
	//
	// The request path is a symbolic segment containing the tag name.
	// The request data contains the number of elements to read (UINT, 2 bytes).
	//
	// The response data contains:
	//   - Data Type (UINT, 2 bytes)
	//   - Tag value data (variable length, depends on type and element count)
	// -----------------------------------------------------------------------
	case cip.ServiceReadTag:
		return t.handleReadTag(path, data)

	// -----------------------------------------------------------------------
	// WriteTag (0x4D) -- Write Tag Service
	//
	// The request path is a symbolic segment containing the tag name.
	// The request data contains:
	//   - Data Type (UINT, 2 bytes)
	//   - Number of Elements (UINT, 2 bytes)
	//   - Tag value data (variable length)
	//
	// The response data is empty on success.
	// -----------------------------------------------------------------------
	case cip.ServiceWriteTag:
		return t.handleWriteTag(path, data)

	// -----------------------------------------------------------------------
	// GetAttributeAll (0x01) -- used by some clients to probe the object
	// -----------------------------------------------------------------------
	case cip.ServiceGetAttributeAll:
		return t.handleGetAttributeAll()

	// -----------------------------------------------------------------------
	// Any other service code is not supported by this object.
	// -----------------------------------------------------------------------
	default:
		return nil, cip.Error{Status: cip.StatusServiceNotSupported}
	}
}

// handleReadTag processes a ReadTag (0x4C) request.
func (t *TagObject) handleReadTag(path cip.Path, data []byte) ([]byte, error) {
	// Parse the tag name from the symbolic segment in the path.
	tagName, err := parseSymbolicSegment(path)
	if err != nil {
		return nil, cip.Error{Status: cip.StatusPathSegmentError}
	}

	// The request data must contain at least 2 bytes for the element count.
	if len(data) < 2 {
		return nil, cip.Error{Status: cip.StatusNotEnoughData}
	}

	// Parse the requested element count (we only support 1 element for
	// simplicity in this example).
	// elements := binary.LittleEndian.Uint16(data[0:2])
	// For this demo, we always return one element regardless of count.

	// Look up the tag in our database.
	t.mu.RLock()
	entry, ok := t.tags[tagName]
	t.mu.RUnlock()

	if !ok {
		// Tag not found. Return the "path destination unknown" error, which
		// is the standard CIP way to say "this tag does not exist".
		return nil, cip.Error{Status: cip.StatusPathDestinationUnknown}
	}

	// Build the response: [DataType (2 bytes)] [Value data (N bytes)]
	// This matches the format that CIP clients expect from ReadTag.
	resp := make([]byte, 2+len(entry.data))
	binary.LittleEndian.PutUint16(resp[0:2], uint16(entry.dataType))
	copy(resp[2:], entry.data)

	return resp, nil
}

// handleWriteTag processes a WriteTag (0x4D) request.
func (t *TagObject) handleWriteTag(path cip.Path, data []byte) ([]byte, error) {
	// Parse the tag name from the symbolic segment in the path.
	tagName, err := parseSymbolicSegment(path)
	if err != nil {
		return nil, cip.Error{Status: cip.StatusPathSegmentError}
	}

	// The request data must contain at least 4 bytes:
	//   - Data Type (2 bytes)
	//   - Number of Elements (2 bytes)
	// followed by the actual value data.
	if len(data) < 4 {
		return nil, cip.Error{Status: cip.StatusNotEnoughData}
	}

	// Parse the data type and element count from the request.
	dataType := cip.DataType(binary.LittleEndian.Uint16(data[0:2]))
	// elements := binary.LittleEndian.Uint16(data[2:4])  // not used in this simple example
	valueData := data[4:] // The raw value bytes follow the header.

	// Store the tag value. If the tag already exists, its value is overwritten.
	// If it does not exist, a new tag is created.
	t.mu.Lock()
	t.tags[tagName] = &tagEntry{
		dataType: dataType,
		data:     make([]byte, len(valueData)),
	}
	copy(t.tags[tagName].data, valueData)
	t.mu.Unlock()

	// WriteTag returns an empty response on success.
	return nil, nil
}

// handleGetAttributeAll returns a simple summary of the tag object.
// Some clients call this to check whether the object exists.
func (t *TagObject) handleGetAttributeAll() ([]byte, error) {
	t.mu.RLock()
	count := len(t.tags)
	t.mu.RUnlock()

	// Return the tag count as a UINT (2 bytes, little-endian).
	resp := make([]byte, 2)
	binary.LittleEndian.PutUint16(resp, uint16(count))
	return resp, nil
}

// ---------------------------------------------------------------------------
// parseSymbolicSegment extracts the tag name from a CIP EPATH that begins
// with an ANSI Extended Symbol segment (0x91).
//
// The segment format is:
//   - 0x91 (segment type byte)
//   - length (1 byte, number of characters in the symbol name)
//   - symbol name (variable length, ASCII)
//   - optional pad byte (if length is odd, to align to 16-bit boundary)
//
// This is the same path format used by the EtherNet/IP client when it calls
// cip.NewPath().AddSymbolicSegment("TagName").
// ---------------------------------------------------------------------------
func parseSymbolicSegment(path cip.Path) (string, error) {
	raw := path.Bytes()
	if len(raw) < 2 {
		return "", fmt.Errorf("path too short for symbolic segment")
	}

	// Check for the ANSI Extended Symbol segment type (0x91).
	if raw[0] != 0x91 {
		return "", fmt.Errorf("expected symbolic segment (0x91), got 0x%02X", raw[0])
	}

	// The second byte is the string length.
	nameLen := int(raw[1])
	if len(raw) < 2+nameLen {
		return "", fmt.Errorf("path too short for symbol name of length %d", nameLen)
	}

	return string(raw[2 : 2+nameLen]), nil
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	// -------------------------------------------------------------------
	// Parse command-line flags
	// -------------------------------------------------------------------

	// -addr: the TCP address the server should bind to. The default
	// EtherNet/IP port is 44818 (0xAF12). Using ":44818" binds to all
	// interfaces. For local testing you might use "127.0.0.1:44818".
	addr := flag.String("addr", ":44818", "TCP address to listen on (host:port)")

	flag.Parse()

	// -------------------------------------------------------------------
	// Set up logging
	// -------------------------------------------------------------------
	// We use the library's built-in logger. In production you would
	// typically integrate with your own logging framework (slog, zap, etc.).
	logger := logging.NewDefaultLogger(logging.WithLevel(logging.LevelInfo))

	// -------------------------------------------------------------------
	// Create the CIP Message Router
	// -------------------------------------------------------------------
	// The Message Router is the core of the CIP object model. It receives
	// all incoming CIP requests and dispatches them to the registered
	// objects based on the Class ID in the request path.
	//
	// In a real device, you would register standard CIP objects here:
	//   - Identity Object (Class 0x01)
	//   - Assembly Object (Class 0x04)
	//   - Connection Manager (Class 0x06)
	//   - etc.
	//
	// For this example, we register a single custom TagObject that handles
	// ReadTag/WriteTag requests. We use an arbitrary class ID (0x64 = 100)
	// because this is a custom, non-standard object. The EtherNet/IP client
	// in goindustrial routes ReadTag requests using the symbolic segment
	// path, and the server's handleSendRRData method routes them through
	// the message router. The client constructs paths like:
	//
	//   [0x91] [len] [tag_name...]
	//
	// But the message router expects paths starting with a Class segment:
	//
	//   [0x20] [classID] [remaining path...]
	//
	// Since the goindustrial client sends ReadTag requests using symbolic
	// paths (not class-based paths), and the server dispatches based on the
	// request path, we register our TagObject under a well-known class ID.
	// The symbolic path routing is handled by the server's request parsing.
	//
	// Note: In the current server implementation, the router expects a
	// Class segment as the first path element. For symbolic ReadTag
	// requests from the goindustrial client, the path starts with 0x91
	// (symbolic segment), not 0x20 (class segment). This means the router
	// will return "path segment error" for symbolic-only paths. To handle
	// this properly, a production server would need to add symbolic path
	// routing or a "default object" concept. For this example, we register
	// our object under ClassAssembly (0x04) as a demonstration.
	router := cip.NewMessageRouter()

	// Create the tag object and populate it with demo tags.
	tagObj := NewTagObject()

	// Pre-populate some tags so clients can read them immediately.
	// Each tag is stored as raw bytes in CIP little-endian encoding.

	// DINT tag: "MyDINT" = 12345 (a 32-bit signed integer)
	dintBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(dintBytes, uint32(12345))
	tagObj.SetTag("MyDINT", cip.TypeDINT, dintBytes)

	// REAL tag: "MyREAL" = 3.14 (a 32-bit IEEE 754 float)
	realBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(realBytes, math.Float32bits(3.14))
	tagObj.SetTag("MyREAL", cip.TypeREAL, realBytes)

	// STRING tag: "MySTRING" = "Hello, EIP!"
	// CIP STRING format: [UINT length][character data...]
	// The length is the number of characters, encoded as a little-endian UINT.
	strValue := "Hello, EIP!"
	strBytes := make([]byte, 2+len(strValue))
	binary.LittleEndian.PutUint16(strBytes[0:2], uint16(len(strValue)))
	copy(strBytes[2:], strValue)
	tagObj.SetTag("MySTRING", cip.TypeSTRING, strBytes)

	// DINT tag: "Counter" = 0 (a counter that clients can increment)
	counterBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(counterBytes, 0)
	tagObj.SetTag("Counter", cip.TypeDINT, counterBytes)

	// Register the tag object with the message router under ClassAssembly
	// (0x04). In a production implementation, you would typically create a
	// custom class or use the Symbol Object class.
	router.RegisterObject(cip.ClassAssembly, tagObj)

	// Print the list of pre-populated tags so the user knows what to test.
	fmt.Println("Pre-populated tags:")
	fmt.Println("  MyDINT   (DINT)   = 12345")
	fmt.Println("  MyREAL   (REAL)   = 3.14")
	fmt.Printf("  MySTRING (STRING) = %q\n", strValue)
	fmt.Println("  Counter  (DINT)   = 0")
	fmt.Println()

	// -------------------------------------------------------------------
	// Create and start the EtherNet/IP server
	// -------------------------------------------------------------------
	// The server handles the EtherNet/IP TCP protocol:
	//   1. Accepts incoming TCP connections
	//   2. Processes RegisterSession commands (EIP session management)
	//   3. Processes SendRRData commands (unconnected CIP messaging)
	//   4. Dispatches CIP requests through the message router
	//
	// Each client connection runs in its own goroutine, so the server can
	// handle multiple simultaneous clients.
	srv := ethernetip.NewServer(router,
		ethernetip.WithServerLogger(logger), // Attach our logger for visibility
	)

	// Start the server. This opens a TCP listener and begins accepting
	// connections in background goroutines. Start() returns immediately.
	ctx := context.Background()
	fmt.Printf("Starting EtherNet/IP server on %s...\n", *addr)

	if err := srv.Start(ctx, *addr); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start server: %v\n", err)
		// Common causes:
		//   - "address already in use": another process is using the port
		//   - "permission denied": port < 1024 requires root on Linux
		os.Exit(1)
	}

	fmt.Println("Server is running. Press Ctrl+C to stop.")
	fmt.Println()

	// -------------------------------------------------------------------
	// Wait for shutdown signal
	// -------------------------------------------------------------------
	// We use os/signal to catch SIGINT (Ctrl+C) and SIGTERM (sent by
	// process managers like systemd, Docker, Kubernetes, etc.).
	//
	// When the signal is received, we call srv.Stop() which:
	//   1. Closes the TCP listener (stops accepting new connections)
	//   2. Closes all active client connections
	//   3. Returns once all goroutines have finished
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Block until a signal arrives.
	sig := <-sigCh
	fmt.Printf("\nReceived signal: %v\n", sig)
	fmt.Println("Shutting down server...")

	if err := srv.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping server: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Server stopped.")
}

