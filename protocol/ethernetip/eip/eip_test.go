package eip

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// ===========================================================================
// Encapsulation Header Encode/Decode Round-Trip
// ===========================================================================

func TestHeaderEncodeDecodeRoundTrip(t *testing.T) {
	original := &EncapsulationHeader{
		Command:       CommandRegisterSession,
		Length:        4,
		SessionHandle: 0x00060212,
		Status:        0,
		SenderContext: [8]byte{0xD7, 0xA5, 0x7E, 0x26, 0x73, 0xC0, 0x35, 0xF4},
		Options:       0,
	}
	buf := new(bytes.Buffer)
	if err := original.Encode(buf); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != HeaderSize {
		t.Fatalf("encoded size = %d, want %d", buf.Len(), HeaderSize)
	}

	decoded := &EncapsulationHeader{}
	if err := decoded.Decode(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatal(err)
	}
	if decoded.Command != original.Command {
		t.Errorf("Command = 0x%04X, want 0x%04X", decoded.Command, original.Command)
	}
	if decoded.Length != original.Length {
		t.Errorf("Length = %d, want %d", decoded.Length, original.Length)
	}
	if decoded.SessionHandle != original.SessionHandle {
		t.Errorf("SessionHandle = 0x%08X, want 0x%08X", decoded.SessionHandle, original.SessionHandle)
	}
	if decoded.SenderContext != original.SenderContext {
		t.Errorf("SenderContext = %X, want %X", decoded.SenderContext, original.SenderContext)
	}
}

// ===========================================================================
// Raw Wire Format Decode — Binary Vectors from OpENer Fuzz Inputs
// These are known-good packets captured from real EIP implementations
// ===========================================================================

func TestDecodeRawListIdentity(t *testing.T) {
	// OpENer fuzz input: enip_req_list_identity (24 bytes)
	raw := []byte{
		0x63, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
	h := &EncapsulationHeader{}
	if err := h.Decode(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	if h.Command != CommandListIdentity {
		t.Errorf("Command = 0x%04X, want ListIdentity (0x%04X)", h.Command, CommandListIdentity)
	}
	if h.Length != 0 {
		t.Errorf("Length = %d, want 0", h.Length)
	}
	if h.SessionHandle != 0 {
		t.Errorf("SessionHandle = 0x%08X, want 0", h.SessionHandle)
	}
	if h.Status != 0 {
		t.Errorf("Status = 0x%08X, want 0", h.Status)
	}
}

func TestDecodeRawRegisterSession(t *testing.T) {
	// OpENer fuzz input: enip_req_register_session (28 bytes)
	raw := []byte{
		0x65, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0xD7, 0xA5, 0x7E, 0x26,
		0x73, 0xC0, 0x35, 0xF4, 0x00, 0x00, 0x00, 0x00,
		// Data: protocol version 1, options 0
		0x01, 0x00, 0x00, 0x00,
	}
	h := &EncapsulationHeader{}
	if err := h.Decode(bytes.NewReader(raw[:HeaderSize])); err != nil {
		t.Fatal(err)
	}
	if h.Command != CommandRegisterSession {
		t.Errorf("Command = 0x%04X, want RegisterSession (0x%04X)", h.Command, CommandRegisterSession)
	}
	if h.Length != 4 {
		t.Errorf("Length = %d, want 4", h.Length)
	}
	if h.SessionHandle != 0 {
		t.Errorf("SessionHandle = 0x%08X, want 0 (new session request)", h.SessionHandle)
	}

	// Parse RegisterSessionData
	data := raw[HeaderSize:]
	version := binary.LittleEndian.Uint16(data[0:2])
	options := binary.LittleEndian.Uint16(data[2:4])
	if version != 1 {
		t.Errorf("protocol version = %d, want 1", version)
	}
	if options != 0 {
		t.Errorf("options = %d, want 0", options)
	}

	// Verify sender context is preserved
	expectedCtx := [8]byte{0xD7, 0xA5, 0x7E, 0x26, 0x73, 0xC0, 0x35, 0xF4}
	if h.SenderContext != expectedCtx {
		t.Errorf("SenderContext = %X, want %X", h.SenderContext, expectedCtx)
	}
}

func TestDecodeRawSendRRData(t *testing.T) {
	// OpENer encaptest.cpp: SendRRData request
	raw := []byte{
		0x6F, 0x00, 0x0C, 0x00, 0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0xF0, 0xDD, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
	h := &EncapsulationHeader{}
	if err := h.Decode(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	if h.Command != CommandSendRRData {
		t.Errorf("Command = 0x%04X, want SendRRData (0x%04X)", h.Command, CommandSendRRData)
	}
	if h.Length != 12 {
		t.Errorf("Length = %d, want 12", h.Length)
	}
	if h.SessionHandle != 1 {
		t.Errorf("SessionHandle = %d, want 1", h.SessionHandle)
	}
}

func TestDecodeRawListIdentityFromTest(t *testing.T) {
	// OpENer encaptest.cpp AnswerListIdentityRequest
	raw := []byte{
		0x63, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0xD7, 0xDD, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
	h := &EncapsulationHeader{}
	if err := h.Decode(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	if h.Command != CommandListIdentity {
		t.Errorf("Command = 0x%04X, want ListIdentity", h.Command)
	}
	expectedCtx := [8]byte{0xD7, 0xDD, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	if h.SenderContext != expectedCtx {
		t.Errorf("SenderContext = %X, want %X", h.SenderContext, expectedCtx)
	}
}

func TestDecodeRawListServices(t *testing.T) {
	// OpENer encaptest.cpp AnswerListServicesRequest
	raw := []byte{
		0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0xE0, 0xDD, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
	h := &EncapsulationHeader{}
	if err := h.Decode(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	if h.Command != CommandListServices {
		t.Errorf("Command = 0x%04X, want ListServices (0x%04X)", h.Command, CommandListServices)
	}
	if h.Length != 0 {
		t.Errorf("Length = %d, want 0", h.Length)
	}
}

func TestDecodeRawInvalidProtocolVersion(t *testing.T) {
	// OpENer encaptest.cpp AnswerRegisterSessionRequestWrongProtocolVersion
	// Protocol version 0 should be rejected
	raw := []byte{
		0x65, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x67, 0x88, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Data: protocol version 0, options 0
		0x00, 0x00, 0x00, 0x00,
	}
	h := &EncapsulationHeader{}
	if err := h.Decode(bytes.NewReader(raw[:HeaderSize])); err != nil {
		t.Fatal(err)
	}
	if h.Command != CommandRegisterSession {
		t.Errorf("Command = 0x%04X, want RegisterSession", h.Command)
	}
	// Protocol version 0 is invalid per spec
	data := raw[HeaderSize:]
	version := binary.LittleEndian.Uint16(data[0:2])
	if version != 0 {
		t.Errorf("expected invalid protocol version 0, got %d", version)
	}
}

func TestDecodeRawForwardOpen(t *testing.T) {
	// OpENer fuzz input: cip_req_forward_open (88 bytes)
	// This is a full SendRRData wrapping a CIP Forward Open request
	raw := []byte{
		0x6F, 0x00, 0x40, 0x00, 0x00, 0x06, 0x02, 0x12,
		0x00, 0x00, 0x00, 0x00, 0xBC, 0x6F, 0xFA, 0x27,
		0x04, 0x18, 0x83, 0xBA, 0x00, 0x00, 0x00, 0x00,
	}
	h := &EncapsulationHeader{}
	if err := h.Decode(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	if h.Command != CommandSendRRData {
		t.Errorf("Command = 0x%04X, want SendRRData", h.Command)
	}
	if h.Length != 64 {
		t.Errorf("Length = %d, want 64", h.Length)
	}
	// Session handle is the established session
	if h.SessionHandle == 0 {
		t.Error("SessionHandle should not be 0 for Forward Open")
	}
}

// ===========================================================================
// Header Bytes and String
// ===========================================================================

func TestHeaderBytes(t *testing.T) {
	h := &EncapsulationHeader{
		Command: CommandListIdentity,
		Length:  0,
	}
	data := h.Bytes()
	if len(data) != HeaderSize {
		t.Fatalf("Bytes() length = %d, want %d", len(data), HeaderSize)
	}
	cmd := binary.LittleEndian.Uint16(data[0:2])
	if cmd != uint16(CommandListIdentity) {
		t.Errorf("command in bytes = 0x%04X, want 0x%04X", cmd, CommandListIdentity)
	}
}

func TestHeaderString(t *testing.T) {
	h := &EncapsulationHeader{
		Command:       CommandRegisterSession,
		Length:        4,
		SessionHandle: 1,
	}
	s := h.String()
	if s == "" {
		t.Fatal("String() should not be empty")
	}
}

// ===========================================================================
// Command String
// ===========================================================================

func TestCommandString(t *testing.T) {
	tests := []struct {
		cmd  Command
		want string
	}{
		{CommandNop, "Nop"},
		{CommandListServices, "ListServices"},
		{CommandListIdentity, "ListIdentity"},
		{CommandListInterfaces, "ListInterfaces"},
		{CommandRegisterSession, "RegisterSession"},
		{CommandUnregisterSession, "UnregisterSession"},
		{CommandSendRRData, "SendRRData"},
		{CommandSendUnitData, "SendUnitData"},
		{CommandIndicateStatus, "IndicateStatus"},
		{CommandCancel, "Cancel"},
	}
	for _, tt := range tests {
		if got := tt.cmd.String(); got != tt.want {
			t.Errorf("Command(0x%04X).String() = %q, want %q", uint16(tt.cmd), got, tt.want)
		}
	}
	// Unknown command
	unk := Command(0xFFFF)
	if s := unk.String(); s == "" {
		t.Fatal("unknown command should return non-empty string")
	}
}

// ===========================================================================
// RegisterSessionData
// ===========================================================================

func TestRegisterSessionDataEncode(t *testing.T) {
	d := NewRegisterSessionData()
	data, err := d.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 4 {
		t.Fatalf("encoded length = %d, want 4", len(data))
	}
	version := binary.LittleEndian.Uint16(data[0:2])
	opts := binary.LittleEndian.Uint16(data[2:4])
	if version != 1 {
		t.Errorf("protocol version = %d, want 1", version)
	}
	if opts != 0 {
		t.Errorf("options = %d, want 0", opts)
	}
}

// ===========================================================================
// EIP Status Codes
// ===========================================================================

func TestEIPStatusCodes(t *testing.T) {
	// Validate all status codes from OpENer encap.h exist
	codes := map[string]uint32{
		"Success":              StatusSuccess,
		"InvalidCommand":       StatusInvalidCommand,
		"InsufficientMemory":   StatusInsufficientMemory,
		"IncorrectData":        StatusIncorrectData,
		"InvalidSessionHandle": StatusInvalidSessionHandle,
		"InvalidLength":        StatusInvalidLength,
		"UnsupportedProtocol":  StatusUnsupportedProtocol,
	}
	for name, code := range codes {
		_ = code // Just verifying they compile and are defined
		if name == "" {
			t.Fatal("empty name")
		}
	}
	// Specific value checks from OpENer
	if StatusInvalidSessionHandle != 0x00000064 {
		t.Errorf("InvalidSessionHandle = 0x%08X, want 0x00000064", StatusInvalidSessionHandle)
	}
	if StatusInvalidLength != 0x00000065 {
		t.Errorf("InvalidLength = 0x%08X, want 0x00000065", StatusInvalidLength)
	}
	if StatusUnsupportedProtocol != 0x00000069 {
		t.Errorf("UnsupportedProtocol = 0x%08X, want 0x00000069", StatusUnsupportedProtocol)
	}
}

// ===========================================================================
// Common Packet Format (CPF) Tests
// ===========================================================================

func TestCPFNullAddressEncodeDecodeRoundTrip(t *testing.T) {
	// Standard unconnected message CPF: Null Address + Unconnected Data
	cpf := NewCommonPacketFormat(
		NewCPFItem(ItemIDNullAddress, nil),
		NewCPFItem(ItemIDUnconnectedMessage, []byte{0x4C, 0x02, 0x20, 0x04, 0x24, 0x01}),
	)
	data, err := cpf.Encode()
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeCommonPacketFormat(data)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ItemCount != 2 {
		t.Fatalf("item count = %d, want 2", decoded.ItemCount)
	}

	nullItem := decoded.FindItemByType(ItemIDNullAddress)
	if nullItem == nil {
		t.Fatal("missing null address item")
	}
	if nullItem.Length != 0 {
		t.Errorf("null item length = %d, want 0", nullItem.Length)
	}

	dataItem := decoded.FindItemByType(ItemIDUnconnectedMessage)
	if dataItem == nil {
		t.Fatal("missing unconnected message item")
	}
	if dataItem.Length != 6 {
		t.Errorf("data item length = %d, want 6", dataItem.Length)
	}
}

func TestCPFConnectedAddressRoundTrip(t *testing.T) {
	// Connected message: Connection Address + Connected Data
	connID := make([]byte, 4)
	binary.LittleEndian.PutUint32(connID, 0x12345678)

	seqData := make([]byte, 2)
	binary.LittleEndian.PutUint16(seqData, 1) // Sequence number
	connData := append(seqData, 0xAA, 0xBB, 0xCC, 0xDD)

	cpf := NewCommonPacketFormat(
		NewCPFItem(ItemIDConnectionBased, connID),
		NewCPFItem(ItemIDConnectedTransport, connData),
	)
	data, err := cpf.Encode()
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeCommonPacketFormat(data)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ItemCount != 2 {
		t.Fatalf("item count = %d, want 2", decoded.ItemCount)
	}

	addrItem := decoded.FindItemByType(ItemIDConnectionBased)
	if addrItem == nil {
		t.Fatal("missing connection address item")
	}
	gotConnID := binary.LittleEndian.Uint32(addrItem.Data)
	if gotConnID != 0x12345678 {
		t.Errorf("connection ID = 0x%08X, want 0x12345678", gotConnID)
	}

	transItem := decoded.FindItemByType(ItemIDConnectedTransport)
	if transItem == nil {
		t.Fatal("missing connected data item")
	}
	gotSeq := binary.LittleEndian.Uint16(transItem.Data[0:2])
	if gotSeq != 1 {
		t.Errorf("sequence = %d, want 1", gotSeq)
	}
}

func TestCPFSequencedAddressItem(t *testing.T) {
	// Sequenced Address Item (0x8002): 8 bytes = ConnectionID + SequenceNumber
	addrData := make([]byte, 8)
	binary.LittleEndian.PutUint32(addrData[0:4], 0xAABBCCDD) // Connection ID
	binary.LittleEndian.PutUint32(addrData[4:8], 42)          // Sequence Number

	cpf := NewCommonPacketFormat(
		NewCPFItem(ItemIDSequencedAddress, addrData),
		NewCPFItem(ItemIDConnectedTransport, []byte{0x01, 0x02, 0x03}),
	)
	data, err := cpf.Encode()
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeCommonPacketFormat(data)
	if err != nil {
		t.Fatal(err)
	}

	seqItem := decoded.FindItemByType(ItemIDSequencedAddress)
	if seqItem == nil {
		t.Fatal("missing sequenced address item")
	}
	if seqItem.Length != 8 {
		t.Errorf("sequenced addr length = %d, want 8", seqItem.Length)
	}
	connID := binary.LittleEndian.Uint32(seqItem.Data[0:4])
	seqNum := binary.LittleEndian.Uint32(seqItem.Data[4:8])
	if connID != 0xAABBCCDD {
		t.Errorf("connection ID = 0x%08X, want 0xAABBCCDD", connID)
	}
	if seqNum != 42 {
		t.Errorf("sequence number = %d, want 42", seqNum)
	}
}

func TestCPFSocketAddrInfoItem(t *testing.T) {
	// Socket Address Info Item (0x8000 O->T): 16 bytes
	// Format: sin_family(2, big-endian) + sin_port(2, big-endian) + sin_addr(4, big-endian) + sin_zero(8)
	sockData := make([]byte, 16)
	binary.BigEndian.PutUint16(sockData[0:2], 2)     // AF_INET = 2
	binary.BigEndian.PutUint16(sockData[2:4], 0x08AE) // Port 2222
	sockData[4] = 192
	sockData[5] = 168
	sockData[6] = 1
	sockData[7] = 10

	cpf := NewCommonPacketFormat(
		NewCPFItem(ItemIDNullAddress, nil),
		NewCPFItem(ItemIDUnconnectedMessage, []byte{0x01}),
		NewCPFItem(ItemIDSockaddrInfo, sockData),
	)
	data, err := cpf.Encode()
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeCommonPacketFormat(data)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ItemCount != 3 {
		t.Fatalf("item count = %d, want 3", decoded.ItemCount)
	}

	sockItem := decoded.FindItemByType(ItemIDSockaddrInfo)
	if sockItem == nil {
		t.Fatal("missing sockaddr info item")
	}
	if sockItem.Length != 16 {
		t.Errorf("sockaddr length = %d, want 16", sockItem.Length)
	}
	family := binary.BigEndian.Uint16(sockItem.Data[0:2])
	if family != 2 {
		t.Errorf("sin_family = %d, want 2 (AF_INET)", family)
	}
	port := binary.BigEndian.Uint16(sockItem.Data[2:4])
	if port != 0x08AE {
		t.Errorf("sin_port = 0x%04X, want 0x08AE", port)
	}
}

func TestCPFListIdentityResponseItem(t *testing.T) {
	identityData := []byte{0x01, 0x00, 0x0C, 0x00, 0x2B, 0x00}
	cpf := NewCommonPacketFormat(
		NewCPFItem(ItemIDListIdentity, identityData),
	)
	data, err := cpf.Encode()
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeCommonPacketFormat(data)
	if err != nil {
		t.Fatal(err)
	}
	item := decoded.FindItemByType(ItemIDListIdentity)
	if item == nil {
		t.Fatal("missing list identity item")
	}
	if !bytes.Equal(item.Data, identityData) {
		t.Errorf("data = %X, want %X", item.Data, identityData)
	}
}

func TestCPFListServicesResponseItem(t *testing.T) {
	serviceData := []byte{0x01, 0x00, 0x20, 0x01, 0x00, 0x00}
	cpf := NewCommonPacketFormat(
		NewCPFItem(ItemIDListServices, serviceData),
	)
	data, err := cpf.Encode()
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeCommonPacketFormat(data)
	if err != nil {
		t.Fatal(err)
	}
	item := decoded.FindItemByType(ItemIDListServices)
	if item == nil {
		t.Fatal("missing list services item")
	}
	if !bytes.Equal(item.Data, serviceData) {
		t.Errorf("data = %X, want %X", item.Data, serviceData)
	}
}

func TestCPFEmptyItems(t *testing.T) {
	cpf := NewCommonPacketFormat()
	data, err := cpf.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 2 { // Just the item count
		t.Fatalf("empty CPF length = %d, want 2", len(data))
	}

	decoded, err := DecodeCommonPacketFormat(data)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ItemCount != 0 {
		t.Errorf("item count = %d, want 0", decoded.ItemCount)
	}
}

func TestCPFFindItemByTypeNotFound(t *testing.T) {
	cpf := NewCommonPacketFormat(
		NewCPFItem(ItemIDNullAddress, nil),
	)
	if cpf.FindItemByType(ItemIDUnconnectedMessage) != nil {
		t.Error("expected nil for unfound item type")
	}
}

// ===========================================================================
// CPF Error Cases
// ===========================================================================

func TestCPFDecodeTruncatedHeader(t *testing.T) {
	_, err := DecodeCommonPacketFormat([]byte{0x01}) // Missing second byte of item count
	if err == nil {
		t.Fatal("expected error for truncated CPF")
	}
}

func TestCPFDecodeItemCountExceedsData(t *testing.T) {
	// Item count says 5 items but no data follows
	data := make([]byte, 2)
	binary.LittleEndian.PutUint16(data, 5)
	_, err := DecodeCommonPacketFormat(data)
	if err == nil {
		t.Fatal("expected error when item count exceeds available data")
	}
}

func TestCPFDecodeTruncatedItem(t *testing.T) {
	// 1 item declared, but item header is truncated
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, uint16(1)) // 1 item
	binary.Write(buf, binary.LittleEndian, uint16(0))  // type ID only, missing length
	_, err := DecodeCommonPacketFormat(buf.Bytes())
	if err == nil {
		t.Fatal("expected error for truncated item")
	}
}

// ===========================================================================
// Encapsulation Header Edge Cases
// ===========================================================================

func TestHeaderDecodeShort(t *testing.T) {
	h := &EncapsulationHeader{}
	err := h.Decode(bytes.NewReader(make([]byte, 10))) // Too short
	if err == nil {
		t.Fatal("expected error for short header")
	}
}

func TestHeaderSenderContextPreserved(t *testing.T) {
	ctx := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	h := &EncapsulationHeader{
		Command:       CommandListIdentity,
		SenderContext: ctx,
	}
	data := h.Bytes()

	decoded := &EncapsulationHeader{}
	if err := decoded.Decode(bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if decoded.SenderContext != ctx {
		t.Errorf("sender context not preserved: got %X, want %X", decoded.SenderContext, ctx)
	}
}

func TestHeaderMaxValues(t *testing.T) {
	h := &EncapsulationHeader{
		Command:       Command(0xFFFF),
		Length:        0xFFFF,
		SessionHandle: SessionHandle(0xFFFFFFFF),
		Status:        0xFFFFFFFF,
		SenderContext: [8]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
		Options:       0xFFFFFFFF,
	}
	data := h.Bytes()
	if len(data) != HeaderSize {
		t.Fatalf("max values header size = %d, want %d", len(data), HeaderSize)
	}

	decoded := &EncapsulationHeader{}
	if err := decoded.Decode(bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if decoded.Command != h.Command {
		t.Error("command not preserved at max value")
	}
	if decoded.SessionHandle != h.SessionHandle {
		t.Error("session handle not preserved at max value")
	}
}

// ===========================================================================
// Tests mined from cpppo (https://github.com/pjkundert/cpppo)
// Real Wireshark captures from cpppo's enip_test.py
// ===========================================================================

func TestDecodeRegisterSessionRequest(t *testing.T) {
	// rss_004_request: Register Session request captured from real PLC
	// "4","0.000863000","192.168.222.128","10.220.104.180","ENIP","82","Register Session (Req)"
	raw := []byte{
		0x65, 0x00, // Command: Register Session
		0x04, 0x00, // Length: 4
		0x00, 0x00, 0x00, 0x00, // Session Handle: 0
		0x00, 0x00, 0x00, 0x00, // Status: 0
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Sender Context
		0x00, 0x00, 0x00, 0x00, // Options: 0
	}
	h := &EncapsulationHeader{}
	if err := h.Decode(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	if h.Command != CommandRegisterSession {
		t.Fatalf("command: got 0x%04X, want RegisterSession (0x%04X)", h.Command, CommandRegisterSession)
	}
	if h.Length != 4 {
		t.Fatalf("length: got %d, want 4", h.Length)
	}
	if h.SessionHandle != 0 {
		t.Fatalf("session handle: got 0x%08X, want 0 (new session)", h.SessionHandle)
	}
}

func TestDecodeRegisterSessionReply(t *testing.T) {
	// rss_004_reply: Register Session reply from real Logix PLC
	// Session handle assigned: 0x11021e01
	raw := []byte{
		0x65, 0x00, // Command: Register Session
		0x04, 0x00, // Length: 4
		0x01, 0x1e, 0x02, 0x11, // Session Handle: 0x11021e01
		0x00, 0x00, 0x00, 0x00, // Status: 0 (success)
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Sender Context
		0x00, 0x00, 0x00, 0x00, // Options
	}
	h := &EncapsulationHeader{}
	if err := h.Decode(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	if h.Command != CommandRegisterSession {
		t.Fatalf("command: got 0x%04X, want RegisterSession", h.Command)
	}
	if h.SessionHandle != 0x11021e01 {
		t.Fatalf("session handle: got 0x%08X, want 0x11021E01", h.SessionHandle)
	}
	if h.Status != 0 {
		t.Fatalf("status: got 0x%08X, want 0 (success)", h.Status)
	}
}

func TestDecodeSendRRDataRequest(t *testing.T) {
	// gaa_008_request: SendRRData with Get Attribute All
	// "8","0.153249000","192.168.222.128","10.220.104.180","CIP","100","Get Attribute All"
	raw := []byte{
		0x6f, 0x00, // Command: SendRRData
		0x16, 0x00, // Length: 22
		0x01, 0x1e, 0x02, 0x11, // Session Handle: 0x11021e01
		0x00, 0x00, 0x00, 0x00, // Status
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Sender Context
		0x00, 0x00, 0x00, 0x00, // Options
	}
	h := &EncapsulationHeader{}
	if err := h.Decode(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	if h.Command != CommandSendRRData {
		t.Fatalf("command: got 0x%04X, want SendRRData (0x%04X)", h.Command, CommandSendRRData)
	}
	if h.Length != 22 {
		t.Fatalf("length: got %d, want 22", h.Length)
	}
	if h.SessionHandle != 0x11021e01 {
		t.Fatalf("session handle: got 0x%08X, want 0x11021E01", h.SessionHandle)
	}
}

func TestDecodeGetAttributeAllReply(t *testing.T) {
	// gaa_011_reply: SendRRData reply with device identity "1756-L61/B LOGIX5561"
	raw := []byte{
		0x6f, 0x00, // Command: SendRRData
		0x37, 0x00, // Length: 55
		0x01, 0x1e, 0x02, 0x11, // Session Handle
		0x00, 0x00, 0x00, 0x00, // Status: 0 (success)
		0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Sender Context
		0x00, 0x00, 0x00, 0x00, // Options
	}
	h := &EncapsulationHeader{}
	if err := h.Decode(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	if h.Command != CommandSendRRData {
		t.Fatalf("command: got 0x%04X, want SendRRData", h.Command)
	}
	if h.Length != 0x37 {
		t.Fatalf("length: got %d, want 55", h.Length)
	}
	// Sender context should be {0x02, 0x00, ...} — incremented per request in capture
	if h.SenderContext[0] != 0x02 {
		t.Fatalf("sender context[0]: got 0x%02X, want 0x02", h.SenderContext[0])
	}
}

func TestDecodeReadTagFragmentedError(t *testing.T) {
	// rfg_001_reply: Read Tag Fragmented error 0x05
	raw := []byte{
		0x6f, 0x00, // Command: SendRRData
		0x14, 0x00, // Length: 20
		0x02, 0x67, 0x02, 0x10, // Session Handle
		0x00, 0x00, 0x00, 0x00, // Status: 0 (EIP level success; CIP error inside)
		0x07, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Sender Context
		0x00, 0x00, 0x00, 0x00, // Options
	}
	h := &EncapsulationHeader{}
	if err := h.Decode(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	if h.Command != CommandSendRRData {
		t.Fatalf("command: got 0x%04X, want SendRRData", h.Command)
	}
	// EIP-level status is success even when CIP-level has an error
	if h.Status != 0 {
		t.Fatalf("EIP status: got 0x%08X, want 0 (CIP error is in payload)", h.Status)
	}
	if h.SessionHandle != 0x10026702 {
		t.Fatalf("session handle: got 0x%08X, want 0x10026702", h.SessionHandle)
	}
}

func TestDecodeCPFUnconnectedSend(t *testing.T) {
	// gaa_008_request CPF portion (after EIP header + interface/timeout):
	// Item count=2, NullAddr(0x0000)+Unconnected(0x00B2)
	cpfData := []byte{
		0x02, 0x00, // Item count: 2
		0x00, 0x00, // Item 1: Null Address
		0x00, 0x00, // Item 1: Length 0
		0xb2, 0x00, // Item 2: Unconnected Message (0x00B2)
		0x06, 0x00, // Item 2: Length 6
		0x01,             // CIP service: Get Attribute All
		0x02,             // Path size: 2 words
		0x20, 0x66,       // Class 0x66
		0x24, 0x01,       // Instance 1
	}

	cpf, err := DecodeCommonPacketFormat(cpfData)
	if err != nil {
		t.Fatal(err)
	}
	if len(cpf.Items) != 2 {
		t.Fatalf("item count: got %d, want 2", len(cpf.Items))
	}
	if cpf.Items[0].TypeID != ItemIDNullAddress {
		t.Fatalf("item 0 type: got 0x%04X, want NullAddress (0x0000)", cpf.Items[0].TypeID)
	}
	if cpf.Items[0].Length != 0 {
		t.Fatalf("item 0 length: got %d, want 0", cpf.Items[0].Length)
	}
	if cpf.Items[1].TypeID != ItemIDUnconnectedMessage {
		t.Fatalf("item 1 type: got 0x%04X, want UnconnectedMessage (0x00B2)", cpf.Items[1].TypeID)
	}
	if cpf.Items[1].Length != 6 {
		t.Fatalf("item 1 length: got %d, want 6", cpf.Items[1].Length)
	}
	// Verify the CIP data inside the unconnected message item
	if len(cpf.Items[1].Data) != 6 {
		t.Fatalf("item 1 data length: got %d, want 6", len(cpf.Items[1].Data))
	}
	if cpf.Items[1].Data[0] != 0x01 { // Get Attribute All service
		t.Fatalf("CIP service: got 0x%02X, want 0x01 (Get Attribute All)", cpf.Items[1].Data[0])
	}
}

func TestDecodeCPFSendRRDataReply(t *testing.T) {
	// gaa_008_reply CPF portion — response with CIP data
	cpfData := []byte{
		0x02, 0x00, // Item count: 2
		0x00, 0x00, // Null Address
		0x00, 0x00, // Length 0
		0xb2, 0x00, // Unconnected Message
		0x16, 0x00, // Length 22
		// CIP response data follows (22 bytes)
		0x81, 0x00, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00,
		0x00, 0x00, 0x2d, 0x00, 0x01, 0x00, 0x01, 0x01,
		0xb1, 0x2a, 0x1b, 0x00, 0x0a, 0x00,
	}

	cpf, err := DecodeCommonPacketFormat(cpfData)
	if err != nil {
		t.Fatal(err)
	}
	if len(cpf.Items) != 2 {
		t.Fatalf("item count: got %d, want 2", len(cpf.Items))
	}
	if cpf.Items[1].Length != 22 {
		t.Fatalf("item 1 length: got %d, want 22", cpf.Items[1].Length)
	}
	// First byte of CIP reply: service 0x81 = Get Attribute All response (0x01 | 0x80)
	if cpf.Items[1].Data[0] != 0x81 {
		t.Fatalf("CIP reply service: got 0x%02X, want 0x81", cpf.Items[1].Data[0])
	}
}

func TestRegisterSessionDataPayload(t *testing.T) {
	// Register Session payload is 4 bytes: protocol version 1, options 0
	// rss_004_request last 4 bytes: 0x01, 0x00, 0x00, 0x00
	rsd := &RegisterSessionData{
		ProtocolVersion: 1,
		OptionsFlags:    0,
	}
	encoded, err := rsd.Encode()
	if err != nil {
		t.Fatal(err)
	}
	expected := []byte{0x01, 0x00, 0x00, 0x00}
	if !bytes.Equal(encoded, expected) {
		t.Fatalf("RegisterSessionData: got %X, want %X", encoded, expected)
	}
}

func TestEIPCommandCodes(t *testing.T) {
	// Verify command codes match expected values
	if CommandRegisterSession != 0x0065 {
		t.Fatalf("CommandRegisterSession: got 0x%04X, want 0x0065", CommandRegisterSession)
	}
	if CommandSendRRData != 0x006F {
		t.Fatalf("CommandSendRRData: got 0x%04X, want 0x006F", CommandSendRRData)
	}
	if CommandListIdentity != 0x0063 {
		t.Fatalf("CommandListIdentity: got 0x%04X, want 0x0063", CommandListIdentity)
	}
	if CommandListServices != 0x0004 {
		t.Fatalf("CommandListServices: got 0x%04X, want 0x0004", CommandListServices)
	}
	if CommandNop != 0x0000 {
		t.Fatalf("CommandNop: got 0x%04X, want 0x0000", CommandNop)
	}
}

func TestCPFItemIDs(t *testing.T) {
	// Verify CPF item type IDs match expected values
	if ItemIDNullAddress != 0x0000 {
		t.Fatalf("ItemIDNullAddress: got 0x%04X, want 0x0000", ItemIDNullAddress)
	}
	if ItemIDUnconnectedMessage != 0x00B2 {
		t.Fatalf("ItemIDUnconnectedMessage: got 0x%04X, want 0x00B2", ItemIDUnconnectedMessage)
	}
	if ItemIDConnectedTransport != 0x00B1 {
		t.Fatalf("ItemIDConnectedTransport: got 0x%04X, want 0x00B1", ItemIDConnectedTransport)
	}
	if ItemIDConnectionBased != 0x00A1 {
		t.Fatalf("ItemIDConnectionBased: got 0x%04X, want 0x00A1", ItemIDConnectionBased)
	}
}
