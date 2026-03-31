package cip

import "fmt"

// CIP elementary data types. These type aliases mirror the CIP specification
// naming conventions and are used throughout the package for field types in
// protocol structures.

// USINT is an unsigned 8-bit integer (CIP elementary type).
type USINT uint8

// UINT is an unsigned 16-bit integer (CIP elementary type).
type UINT uint16

// UDINT is an unsigned 32-bit integer (CIP elementary type).
type UDINT uint32

// ULINT is an unsigned 64-bit integer (CIP elementary type).
type ULINT uint64

// SINT is a signed 8-bit integer (CIP elementary type).
type SINT int8

// INT is a signed 16-bit integer (CIP elementary type).
type INT int16

// DINT is a signed 32-bit integer (CIP elementary type).
type DINT int32

// LINT is a signed 64-bit integer (CIP elementary type).
type LINT int64

// REAL is a 32-bit IEEE 754 floating-point number (CIP elementary type).
type REAL float32

// LREAL is a 64-bit IEEE 754 floating-point number (CIP elementary type).
type LREAL float64

// BYTE is an 8-bit bit string (CIP elementary type).
type BYTE byte

// WORD is a 16-bit bit string (CIP elementary type).
type WORD uint16

// DWORD is a 32-bit bit string (CIP elementary type).
type DWORD uint32

// LWORD is a 64-bit bit string (CIP elementary type).
type LWORD uint64

// CIP common service codes as defined in the CIP specification (Volume 1,
// Chapter 5). These are used in [MessageRouterRequest] Service fields.
const (
	ServiceGetAttributeAll        USINT = 0x01 // Get_Attribute_All
	ServiceSetAttributeAll        USINT = 0x02 // Set_Attribute_All
	ServiceGetAttributeList       USINT = 0x03 // Get_Attribute_List
	ServiceSetAttributeList       USINT = 0x04 // Set_Attribute_List
	ServiceReset                  USINT = 0x05 // Reset
	ServiceStart                  USINT = 0x06 // Start
	ServiceStop                   USINT = 0x07 // Stop
	ServiceCreate                 USINT = 0x08 // Create
	ServiceDelete                 USINT = 0x09 // Delete
	ServiceMultipleServicePacket  USINT = 0x0A // Multiple_Service_Packet
	ServiceApplyAttributes        USINT = 0x0D // Apply_Attributes
	ServiceGetAttributeSingle     USINT = 0x0E // Get_Attribute_Single
	ServiceSetAttributeSingle     USINT = 0x10 // Set_Attribute_Single
	ServiceFindNextObjectInstance USINT = 0x11 // Find_Next_Object_Instance
	ServiceRestore                USINT = 0x15 // Restore
	ServiceSave                   USINT = 0x16 // Save
	ServiceNop                    USINT = 0x17 // Nop
	ServiceGetMember              USINT = 0x18 // Get_Member
	ServiceSetMember              USINT = 0x19 // Set_Member
	ServiceInsertMember           USINT = 0x1A // Insert_Member
	ServiceRemoveMember           USINT = 0x1B // Remove_Member
	ServiceGroupSync              USINT = 0x1C // Group_Sync
)

// CIP well-known class IDs. These identify standard CIP objects addressable
// via an EPATH class segment.
const (
	ClassIdentity       UINT = 0x01
	ClassMessageRouter  UINT = 0x02
	ClassDeviceNet      UINT = 0x03
	ClassAssembly       UINT = 0x04
	ClassConnection     UINT = 0x05
	ClassConnectionMgr  UINT = 0x06
	ClassRegister       UINT = 0x07
	ClassParameter      UINT = 0x0F
	ClassParameterGroup UINT = 0x10
	ClassGroup          UINT = 0x12
	ClassDiscreteInput  UINT = 0x1D
	ClassDiscreteOutput UINT = 0x1E
	ClassAnalogInput    UINT = 0x1F
	ClassAnalogOutput   UINT = 0x20
	ClassPositionSensor UINT = 0x23
	ClassPositionCtrl   UINT = 0x24
	ClassACDrive        UINT = 0x2A
	ClassMotorOverload  UINT = 0x2C
	ClassControlNet     UINT = 0xF0
	ClassEthernetLink   UINT = 0xF6
	ClassTCPIPInterface UINT = 0xF5
)

// DataType represents a CIP data type code (16-bit). Bit 15 is the array flag.
type DataType uint16

// CIP data type codes used in tag read/write responses and requests. These
// appear as the first two bytes of CIP Read Tag response data.
const (
	TypeBOOL          DataType = 0x00C1
	TypeSINT          DataType = 0x00C2
	TypeINT           DataType = 0x00C3
	TypeDINT          DataType = 0x00C4
	TypeLINT          DataType = 0x00C5
	TypeUSINT         DataType = 0x00C6
	TypeUINT          DataType = 0x00C7
	TypeUDINT         DataType = 0x00C8
	TypeULINT         DataType = 0x00C9
	TypeREAL          DataType = 0x00CA
	TypeLREAL         DataType = 0x00CB
	TypeSTIME         DataType = 0x00CC
	TypeDATE          DataType = 0x00CD
	TypeTIME_OF_DAY   DataType = 0x00CE
	TypeDATE_AND_TIME DataType = 0x00CF
	TypeSTRING        DataType = 0x00D0
	TypeBYTE          DataType = 0x00D1
	TypeWORD          DataType = 0x00D2
	TypeDWORD         DataType = 0x00D3
	TypeLWORD         DataType = 0x00D4
	TypeSTRING2       DataType = 0x00D5
	TypeFTIME         DataType = 0x00D6
	TypeLTIME         DataType = 0x00D7
	TypeITIME         DataType = 0x00D8
	TypeSTRINGN       DataType = 0x00D9
	TypeSHORT_STRING  DataType = 0x00DA
	TypeTIME          DataType = 0x00DB
	TypeEPATH         DataType = 0x00DC
	TypeENGUNIT       DataType = 0x00DD
	TypeSTRINGI       DataType = 0x00DE
	TypeSTRUCT        DataType = 0x02A0 // Common struct type code
)

// CIP general status codes returned in [MessageRouterResponse.GeneralStatus].
const (
	StatusSuccess                USINT = 0x00
	StatusPathDestinationUnknown USINT = 0x05
	StatusPartialTransfer        USINT = 0x06
	StatusAttributeListShortage  USINT = 0x1C
	StatusPathSegmentError       USINT = 0x04
	StatusConnectionFailure      USINT = 0x01
	StatusResourceUnavailable    USINT = 0x02
	StatusInvalidSegmentType     USINT = 0x03 // or 0x04 depending on context
	StatusServiceNotSupported    USINT = 0x08
	StatusInvalidAttributeValue  USINT = 0x09
	StatusAttributeNotSettable   USINT = 0x0E
	StatusPrivilegeViolation     USINT = 0x10
	StatusDeviceStateConflict    USINT = 0x11
	StatusReplyDataTooLarge      USINT = 0x12
	StatusNotEnoughData          USINT = 0x13
	StatusAttributeNotSupported  USINT = 0x14
	StatusTooMuchData            USINT = 0x15
	StatusObjectDoesNotExist     USINT = 0x16
	StatusServiceFragmentation   USINT = 0x2D
)

// Error represents a CIP-level error containing a general status code and
// optional extended status words.
type Error struct {
	Status    USINT
	ExtStatus []UINT // Extended status is usually a list of words
}

func (e Error) Error() string {
	if len(e.ExtStatus) > 0 {
		return fmt.Sprintf("CIP Error: Status=0x%02X ExtStatus=%04X", e.Status, e.ExtStatus)
	}
	return fmt.Sprintf("CIP Error: Status=0x%02X", e.Status)
}

// IsArray returns true if the array bit (bit 15, 0x8000) is set.
func (d DataType) IsArray() bool {
	return (d & 0x8000) != 0
}

// Base returns the base type code with the array flag masked off.
func (d DataType) Base() DataType {
	return d & 0x7FFF // Mask out Array bit (Bit 15)
}

// String returns a human-readable name for the data type (e.g. "DINT",
// "REAL[]"). If the type code is not a built-in CIP type, the registry is
// checked for a custom type registered via [RegisterType] whose TypeCodec
// implements [fmt.Stringer].
func (d DataType) String() string {
	base := d.Base()
	name, ok := typeNames[base]
	if !ok {
		// Check the custom type registry.
		name = lookupTypeName(base)
	}
	if name == "" {
		if d.IsArray() {
			return fmt.Sprintf("UNKNOWN(0x%04X)[]", uint16(base))
		}
		return fmt.Sprintf("UNKNOWN(0x%04X)", uint16(d))
	}

	if d.IsArray() {
		return name + "[]"
	}
	return name
}

var typeNames = map[DataType]string{
	TypeBOOL:          "BOOL",
	TypeSINT:          "SINT",
	TypeINT:           "INT",
	TypeDINT:          "DINT",
	TypeLINT:          "LINT",
	TypeUSINT:         "USINT",
	TypeUINT:          "UINT",
	TypeUDINT:         "UDINT",
	TypeULINT:         "ULINT",
	TypeREAL:          "REAL",
	TypeLREAL:         "LREAL",
	TypeSTIME:         "STIME",
	TypeDATE:          "DATE",
	TypeTIME_OF_DAY:   "TIME_OF_DAY",
	TypeDATE_AND_TIME: "DATE_AND_TIME",
	TypeSTRING:        "STRING",
	TypeBYTE:          "BYTE",
	TypeWORD:          "WORD",
	TypeDWORD:         "DWORD",
	TypeLWORD:         "LWORD",
	TypeSTRING2:       "STRING2",
	TypeFTIME:         "FTIME",
	TypeLTIME:         "LTIME",
	TypeITIME:         "ITIME",
	TypeSTRINGN:       "STRINGN",
	TypeSHORT_STRING:  "SHORT_STRING",
	TypeTIME:          "TIME",
	TypeEPATH:         "EPATH",
	TypeENGUNIT:       "ENGUNIT",
	TypeSTRINGI:       "STRINGI",
	TypeSTRUCT:        "STRUCT",
}
