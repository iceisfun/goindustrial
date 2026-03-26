package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"log"
	"net"
	"time"

	"github.com/iceisfun/goeip/pkg/cip"
	"github.com/iceisfun/goeip/pkg/objects/assembly"
	"github.com/iceisfun/goeip/pkg/objects/connmgr"
	"github.com/iceisfun/goeip/pkg/runtime"
	"github.com/iceisfun/goeip/pkg/session"
	"github.com/iceisfun/goeip/pkg/transport"
)

func main() {
	var (
		addr           = flag.String("addr", "127.0.0.1:44818", "Target TCP address")
		inputAssembly  = flag.Int("input-assembly", 100, "Input Assembly ID (Target -> Originator)")
		outputAssembly = flag.Int("output-assembly", 150, "Output Assembly ID (Originator -> Target)")
		rpi            = flag.Duration("rpi", 100*time.Millisecond, "RPI (Requested Packet Interval)")
	)
	flag.Parse()

	// 1. Connect to Target
	t, err := transport.NewTCPTransport(*addr)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer t.Close()

	sess := session.NewSession(t, nil)
	if err := sess.Register(); err != nil {
		log.Fatalf("Failed to register session: %v", err)
	}
	defer sess.Unregister()

	// 2. Setup Local Runtime (UDP)
	// Scanner listens on a random port or 2222?
	// Usually Scanner listens on 2222 if possible, or any port.
	// Let's try 2222, if busy (Adapter running), use 0 (random).
	// But Adapter is running on 2222.
	// So Scanner must use a different port if on localhost.
	// Let's use 0.

	ao := assembly.NewAssemblyObject()
	rt := runtime.NewRuntime(ao)
	if err := rt.Start(":0"); err != nil {
		log.Fatalf("Failed to start UDP runtime: %v", err)
	}

	// Get Local UDP Port
	// We need to know our port to tell the Target where to send T->O data.
	// But Forward_Open doesn't strictly send the IP/Port.
	// The Target uses the IP from the Forward_Open request (if encapsulated) or just assumes?
	// Actually, Forward_Open has "Network Connection Parameters".
	// But the IP address is usually implicit or configured?
	// In standard EIP, the Target sends T->O to the Originator's IP.
	// The Port is usually 2222.
	// If we are on the same machine, we can't both bind 2222.
	// If Scanner binds 2223, Target needs to know.
	// Standard EIP doesn't easily support non-standard ports for I/O without extra config or multicast.
	// However, for Unicast, the Target sends to the Originator's IP.
	// Does it send to the source port of the O->T packet? No, O->T is UDP.
	// The Target sends to Port 2222 by default.

	// If we are testing on localhost, we have a problem if both want 2222.
	// We might need to run Adapter on 2222 and Scanner on another machine or container.
	// OR, we assume the Adapter sends to the port we specify?
	// Forward_Open doesn't have a field for "My UDP Port".

	// For this demo, let's assume we are running on different IPs or we just test the O->T path (Scanner -> Adapter).
	// Or we can make the Adapter send to the source port of the O->T packet if we implement "Class 1" with "Point-to-Point".
	// But standard says 2222.

	// Let's just start Runtime on :0 and log it.
	// If Adapter sends to 2222, we won't receive it on localhost if Adapter holds 2222.
	// But we can verify O->T (Scanner -> Adapter) works.

	// 3. Send Forward_Open
	// We need to construct the request manually or use a helper.
	// `pkg/objects/connmgr` has the structs but they are internal to the package?
	// No, I exported them in `types.go`.

	// We need to encode ForwardOpenRequest.
	// And wrap it in a MessageRouterRequest (Service 0x54, Class 0x06, Instance 1).

	// Let's create a helper in `cmd/scanner` or just do it inline.

	// Connection IDs
	otConnID := uint32(0x10000001)
	// toConnID := uint32(0) // Target will allocate

	// Network Connection Params
	// O->T: Point-to-Point, Scheduled, Low Priority?
	// Size: 32 bytes (Assembly size) + Header?
	// Let's use 500 bytes size limit.
	// otParams := uint16(0x4000) | 500 // Owner, Fixed? No.
	// 0x4200 ?
	// Let's use 0 (default) for now or check spec.
	// Bit 15: Owner (1)
	// Bit 14: Fixed/Variable (0=Fixed)
	// Bit 13: Priority (0=Low)
	// Bit 9: Type (2=Point-to-Point)
	// 0x4200 = Owner, P2P.

	req := connmgr.ForwardOpenRequest{
		PriorityTimeTick:            0x0A, // 2^10 ms? No, Priority/Tick. 0x03 = 1ms?
		TimeoutTicks:                249,
		OTConnectionID:              cip.UDINT(otConnID),
		TOConnectionID:              0,
		ConnectionSerialNumber:      1234,
		VendorID:                    0x1337,
		OriginatorSerialNumber:      5678,
		ConnectionTimeoutMultiplier: 1, // x4
		Reserved:                    [3]cip.BYTE{0, 0, 0},
		OTRPI:                       cip.UDINT(*rpi / time.Microsecond),
		OTNetworkConnectionParams:   cip.WORD(0x4200 | 36), // 32 data + 4 header
		TORPI:                       cip.UDINT(*rpi / time.Microsecond),
		TONetworkConnectionParams:   cip.WORD(0x4200 | 36),
		TransportTypeTrigger:        0x01, // Cyclic, Direction=Server?
		ConnectionPathSize:          0,    // Calculated later
	}

	// Path:
	// [Class Assembly] [Instance Config] (Optional)
	// [Class Assembly] [Instance Output] (O->T)
	// [Class Assembly] [Instance Input] (T->O)

	// Path Segments:
	// 0x20 0x04 (Class Assembly)
	// 0x24 0x96 (Instance 150 - Output) -> Connection Point O->T
	// 0x2C 0x64 (Instance 100 - Input) -> Connection Point T->O
	// Note: 0x2C is "Connection Point" segment? No, usually we use:
	// Class 4, Instance 150 (Connection Point 1)
	// Class 4, Instance 100 (Connection Point 2)
	// But Forward_Open path is "Application Path".
	// For Exclusive Owner:
	// Port Segment (if needed)
	// Logical Segment: Class 0x04
	// Logical Segment: Instance 0x96 (Configuration) - Optional
	// Logical Segment: Connection Point 0x96 (O->T)
	// Logical Segment: Connection Point 0x64 (T->O)

	// Let's construct path:
	// 0x20 0x04 (Class 4)
	// 0x24 0x96 (Instance 150 - Output)
	// 0x2C 0x64 (Connection Point 100 - Input) - 0x2C is 8-bit connection point?
	// Wait, standard path is:
	// 20 04 24 96 2C 64

	path := []byte{
		0x20, 0x04,
		0x24, byte(*outputAssembly),
		0x2C, byte(*inputAssembly),
	}
	req.ConnectionPathSize = cip.USINT(len(path) / 2)
	req.ConnectionPath = path

	// Encode Request
	// We need a helper to encode ForwardOpenRequest since we only implemented Decode in connmgr.
	// But we can just manually write it here.

	// ... (Encoding logic omitted for brevity, assuming we can do it)
	// Actually, I should have added Encode to connmgr/types.go.
	// But I can't modify it now easily without context switch.
	// I'll just write it manually.

	// Send Request via Session
	// Service 0x54, Path: Class 6, Instance 1
	mrReq := &cip.MessageRouterRequest{
		Service:     connmgr.ServiceForwardOpen,
		RequestPath: cip.Path([]byte{0x20, 0x06, 0x24, 0x01}),
		RequestData: encodeForwardOpen(&req),
	}

	resp, err := sess.SendCIPRequest(mrReq)
	if err != nil {
		log.Fatalf("Forward_Open failed: %v", err)
	}
	if resp.GeneralStatus != cip.StatusSuccess {
		log.Fatalf("Forward_Open error: 0x%02X", resp.GeneralStatus)
	}

	log.Println("Forward_Open Successful!")

	// Parse Response to get T->O Connection ID
	// ...

	// Start Producing
	// Add Connection to Runtime
	conn := &runtime.IOConnection{
		ConnectionID: otConnID, // We produce to this ID (wait, we produce to the ID the Target expects?)
		// In Forward_Open, we specified OTConnectionID. This is the ID the Target uses to consume.
		// So we send with this ID.
		RPI:           *rpi,
		SequenceCount: 0,
		RunIdleHeader: true,
		RemoteAddr:    &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 2222}, // Hardcoded for now
		Assembly:      &assembly.AssemblyInstance{Data: make([]byte, 32)},     // Dummy data
		IsProducer:    true,
	}
	rt.AddConnection(conn)

	// Run for 10 seconds
	time.Sleep(10 * time.Second)

	// Forward_Close
	// ...
}

func encodeForwardOpen(req *connmgr.ForwardOpenRequest) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, req.PriorityTimeTick)
	binary.Write(buf, binary.LittleEndian, req.TimeoutTicks)
	binary.Write(buf, binary.LittleEndian, req.OTConnectionID)
	binary.Write(buf, binary.LittleEndian, req.TOConnectionID)
	binary.Write(buf, binary.LittleEndian, req.ConnectionSerialNumber)
	binary.Write(buf, binary.LittleEndian, req.VendorID)
	binary.Write(buf, binary.LittleEndian, req.OriginatorSerialNumber)
	binary.Write(buf, binary.LittleEndian, req.ConnectionTimeoutMultiplier)
	binary.Write(buf, binary.LittleEndian, req.Reserved)
	binary.Write(buf, binary.LittleEndian, req.OTRPI)
	binary.Write(buf, binary.LittleEndian, req.OTNetworkConnectionParams)
	binary.Write(buf, binary.LittleEndian, req.TORPI)
	binary.Write(buf, binary.LittleEndian, req.TONetworkConnectionParams)
	binary.Write(buf, binary.LittleEndian, req.TransportTypeTrigger)
	binary.Write(buf, binary.LittleEndian, req.ConnectionPathSize)
	buf.Write(req.ConnectionPath)
	return buf.Bytes()
}
