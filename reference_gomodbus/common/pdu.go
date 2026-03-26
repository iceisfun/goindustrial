package common

// PDU (Protocol Data Unit) is the core Modbus message structure
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4 (MODBUS Data Model)
// A PDU consists of a Function Code followed by Function Code specific data
type PDU struct {
	FunctionCode FunctionCode // 1 byte, Ref: Section 6 (MODBUS Function Codes)
	Data         []byte       // Function-specific data
}
