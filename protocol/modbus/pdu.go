package modbus

// PDU (Protocol Data Unit) is the core Modbus message payload, consisting of a
// function code and function-specific data. In Modbus TCP the PDU is wrapped
// inside an MBAP header to form the complete Application Data Unit (ADU).
type PDU struct {
	FunctionCode FunctionCode // 1 byte, Ref: Section 6 (MODBUS Function Codes)
	Data         []byte       // Function-specific data
}
