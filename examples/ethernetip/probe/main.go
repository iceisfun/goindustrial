// Example: ethernetip/probe
//
// Probes an EtherNet/IP device for identity, capabilities, assembly
// instances, CIP objects, network configuration, and optionally Logix
// tags. Useful for discovering I/O connection parameters before
// establishing implicit messaging, or just understanding what a device
// supports.
//
// Usage:
//
//	go run . <IP>
//	go run . <IP> -tags
//	go run . <IP> -assemblies=500 -timeout=10s
package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"math"
	"net"
	"os"
	"strings"
	"time"

	"github.com/iceisfun/goindustrial/protocol/ethernetip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
)

func main() {
	maxAssembly := flag.Int("assemblies", 255, "Max assembly instance to probe")
	probeTags := flag.Bool("tags", false, "Attempt to list tags (Logix controllers only)")
	timeout := flag.Duration("timeout", 5*time.Second, "Connection timeout")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: go run . <PLC_IP> [flags]")
		flag.PrintDefaults()
		os.Exit(1)
	}
	addr := flag.Arg(0)

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	tcpAddr := addr
	if !strings.Contains(tcpAddr, ":") {
		tcpAddr = net.JoinHostPort(tcpAddr, "44818")
	}

	tc, err := ethernetip.NewTCPConn(tcpAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connect to %s: %v\n", addr, err)
		os.Exit(1)
	}
	defer tc.Close()

	sess := ethernetip.NewSession(tc, nil)
	if err := sess.Register(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Register session: %v\n", err)
		os.Exit(1)
	}
	defer sess.Unregister(context.Background())

	cipReq := func(service cip.USINT, path cip.Path, data []byte) (*cip.MessageRouterResponse, error) {
		return sess.SendCIPRequest(ctx, &cip.MessageRouterRequest{
			Service:     service,
			RequestPath: path,
			RequestData: data,
		})
	}

	// ---------------------------------------------------------------
	// Identity Object (Class 0x01, Instance 1)
	// ---------------------------------------------------------------

	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("  DEVICE IDENTITY")
	fmt.Println("═══════════════════════════════════════════════════════════════")

	resp, err := cipReq(cip.ServiceGetAttributeAll, cip.BuildPath(cip.ClassIdentity, 1, 0), nil)
	if err == nil && resp.IsSuccess() && len(resp.ResponseData) >= 15 {
		d := resp.ResponseData
		vendorID := binary.LittleEndian.Uint16(d[0:2])
		devType := binary.LittleEndian.Uint16(d[2:4])
		prodCode := binary.LittleEndian.Uint16(d[4:6])
		revMajor := d[6]
		revMinor := d[7]
		// d[8:10] = status
		status := binary.LittleEndian.Uint16(d[8:10])
		serial := binary.LittleEndian.Uint32(d[10:14])
		nameLen := d[14]
		name := ""
		if int(15+nameLen) <= len(d) {
			name = string(d[15 : 15+nameLen])
		}

		fmt.Printf("  Product Name:    %s\n", name)
		fmt.Printf("  Vendor ID:       %d (%s)\n", vendorID, vendorName(vendorID))
		fmt.Printf("  Device Type:     %d (%s)\n", devType, deviceTypeName(devType))
		fmt.Printf("  Product Code:    %d\n", prodCode)
		fmt.Printf("  Revision:        %d.%d\n", revMajor, revMinor)
		fmt.Printf("  Serial Number:   0x%08X\n", serial)
		fmt.Printf("  Status:          0x%04X\n", status)
	} else {
		fmt.Println("  (identity read failed)")
	}

	// ---------------------------------------------------------------
	// TCP/IP Interface (Class 0xF5, Instance 1)
	// ---------------------------------------------------------------

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("  TCP/IP INTERFACE")
	fmt.Println("═══════════════════════════════════════════════════════════════")

	// Attribute 1: Status
	resp, err = cipReq(cip.ServiceGetAttributeSingle, cip.BuildPath(0xF5, 1, 1), nil)
	if err == nil && resp.IsSuccess() && len(resp.ResponseData) >= 4 {
		status := binary.LittleEndian.Uint32(resp.ResponseData)
		fmt.Printf("  Interface Status: 0x%08X\n", status)
	}

	// Attribute 3: Configuration capability
	resp, err = cipReq(cip.ServiceGetAttributeSingle, cip.BuildPath(0xF5, 1, 3), nil)
	if err == nil && resp.IsSuccess() && len(resp.ResponseData) >= 4 {
		cap := binary.LittleEndian.Uint32(resp.ResponseData)
		fmt.Printf("  Config Capability: 0x%08X", cap)
		var flags []string
		if cap&0x01 != 0 {
			flags = append(flags, "BOOTP")
		}
		if cap&0x02 != 0 {
			flags = append(flags, "DNS")
		}
		if cap&0x04 != 0 {
			flags = append(flags, "DHCP")
		}
		if cap&0x10 != 0 {
			flags = append(flags, "HW-Configurable")
		}
		if cap&0x20 != 0 {
			flags = append(flags, "ACD")
		}
		if len(flags) > 0 {
			fmt.Printf(" [%s]", strings.Join(flags, ", "))
		}
		fmt.Println()
	}

	// Attribute 5: Interface config (IP, mask, gateway)
	// CIP stores IPs as UDINTs in little-endian, so bytes are reversed
	// compared to network byte order. Swap to get a valid net.IP.
	resp, err = cipReq(cip.ServiceGetAttributeSingle, cip.BuildPath(0xF5, 1, 5), nil)
	if err == nil && resp.IsSuccess() && len(resp.ResponseData) >= 12 {
		d := resp.ResponseData
		ip := net.IP{d[3], d[2], d[1], d[0]}
		mask := net.IP{d[7], d[6], d[5], d[4]}
		gw := net.IP{d[11], d[10], d[9], d[8]}
		fmt.Printf("  IP Address:      %s\n", ip)
		fmt.Printf("  Subnet Mask:     %s\n", mask)
		fmt.Printf("  Gateway:         %s\n", gw)
	}

	// Attribute 6: Hostname
	resp, err = cipReq(cip.ServiceGetAttributeSingle, cip.BuildPath(0xF5, 1, 6), nil)
	if err == nil && resp.IsSuccess() && len(resp.ResponseData) >= 2 {
		d := resp.ResponseData
		strLen := binary.LittleEndian.Uint16(d[0:2])
		if int(2+strLen) <= len(d) {
			hostname := string(d[2 : 2+strLen])
			if hostname != "" {
				fmt.Printf("  Hostname:        %s\n", hostname)
			}
		}
	}

	// ---------------------------------------------------------------
	// Ethernet Link (Class 0xF6, Instance 1)
	// ---------------------------------------------------------------

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("  ETHERNET LINK")
	fmt.Println("═══════════════════════════════════════════════════════════════")

	// Attribute 1: Interface speed
	resp, err = cipReq(cip.ServiceGetAttributeSingle, cip.BuildPath(0xF6, 1, 1), nil)
	if err == nil && resp.IsSuccess() && len(resp.ResponseData) >= 4 {
		speed := binary.LittleEndian.Uint32(resp.ResponseData)
		fmt.Printf("  Interface Speed: %d Mbps\n", speed)
	}

	// Attribute 2: Interface flags
	resp, err = cipReq(cip.ServiceGetAttributeSingle, cip.BuildPath(0xF6, 1, 2), nil)
	if err == nil && resp.IsSuccess() && len(resp.ResponseData) >= 4 {
		flags := binary.LittleEndian.Uint32(resp.ResponseData)
		duplex := "half"
		if flags&0x01 != 0 {
			duplex = "full"
		}
		active := "inactive"
		if flags&0x02 != 0 {
			active = "active"
		}
		fmt.Printf("  Link Status:     %s, %s duplex\n", active, duplex)
	}

	// Attribute 3: MAC address
	resp, err = cipReq(cip.ServiceGetAttributeSingle, cip.BuildPath(0xF6, 1, 3), nil)
	if err == nil && resp.IsSuccess() && len(resp.ResponseData) >= 6 {
		mac := resp.ResponseData[:6]
		fmt.Printf("  MAC Address:     %02X:%02X:%02X:%02X:%02X:%02X\n",
			mac[0], mac[1], mac[2], mac[3], mac[4], mac[5])
	}

	// ---------------------------------------------------------------
	// Assembly Object (Class 0x04) - scan instances
	// ---------------------------------------------------------------

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("  ASSEMBLY INSTANCES")
	fmt.Println("═══════════════════════════════════════════════════════════════")

	found := 0
	for inst := uint16(1); inst <= uint16(*maxAssembly); inst++ {
		// Attribute 3 = data, Attribute 4 = size
		path3 := cip.BuildPath(cip.ClassAssembly, cip.UINT(inst), 3)
		resp, err := cipReq(cip.ServiceGetAttributeSingle, path3, nil)
		if err != nil || !resp.IsSuccess() {
			continue
		}

		found++
		data := resp.ResponseData
		fmt.Printf("  Instance %3d:  %3d bytes", inst, len(data))

		// Show data preview
		preview := data
		if len(preview) > 24 {
			preview = preview[:24]
		}
		fmt.Printf("  [% X", preview)
		if len(data) > 24 {
			fmt.Print("...")
		}
		fmt.Print("]")

		// Try to interpret for common drive profiles
		if len(data) == 12 {
			// PowerFlex style: word0=control/status, word1=speed ref/feedback
			w0 := binary.LittleEndian.Uint16(data[0:2])
			w1 := binary.LittleEndian.Uint16(data[2:4])
			fmt.Printf("  (word0=0x%04X word1=0x%04X)", w0, w1)
		}
		fmt.Println()
	}
	if found == 0 {
		fmt.Println("  (none found — device may need I/O module configured)")
	} else {
		fmt.Printf("  Total: %d instances\n", found)
	}

	// ---------------------------------------------------------------
	// Connection Manager (Class 0x06) - check if I/O capable
	// ---------------------------------------------------------------

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("  CONNECTION MANAGER")
	fmt.Println("═══════════════════════════════════════════════════════════════")

	// Attribute 1: Open Requests
	resp, err = cipReq(cip.ServiceGetAttributeSingle, cip.BuildPath(0x06, 1, 1), nil)
	if err == nil && resp.IsSuccess() && len(resp.ResponseData) >= 2 {
		fmt.Printf("  Open Requests:   %d\n", binary.LittleEndian.Uint16(resp.ResponseData))
	}

	// Attribute 2: Open Format Rejects
	resp, err = cipReq(cip.ServiceGetAttributeSingle, cip.BuildPath(0x06, 1, 2), nil)
	if err == nil && resp.IsSuccess() && len(resp.ResponseData) >= 2 {
		fmt.Printf("  Format Rejects:  %d\n", binary.LittleEndian.Uint16(resp.ResponseData))
	}

	// Attribute 3: Open Resource Rejects
	resp, err = cipReq(cip.ServiceGetAttributeSingle, cip.BuildPath(0x06, 1, 3), nil)
	if err == nil && resp.IsSuccess() && len(resp.ResponseData) >= 2 {
		fmt.Printf("  Resource Rejects: %d\n", binary.LittleEndian.Uint16(resp.ResponseData))
	}

	// Attribute 4: Open Other Rejects
	resp, err = cipReq(cip.ServiceGetAttributeSingle, cip.BuildPath(0x06, 1, 4), nil)
	if err == nil && resp.IsSuccess() && len(resp.ResponseData) >= 2 {
		fmt.Printf("  Other Rejects:   %d\n", binary.LittleEndian.Uint16(resp.ResponseData))
	}

	// Attribute 5: Close Requests
	resp, err = cipReq(cip.ServiceGetAttributeSingle, cip.BuildPath(0x06, 1, 5), nil)
	if err == nil && resp.IsSuccess() && len(resp.ResponseData) >= 2 {
		fmt.Printf("  Close Requests:  %d\n", binary.LittleEndian.Uint16(resp.ResponseData))
	}

	// ---------------------------------------------------------------
	// Supported CIP Objects - class scan
	// ---------------------------------------------------------------

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("  CIP OBJECT CLASSES")
	fmt.Println("═══════════════════════════════════════════════════════════════")

	classes := []struct {
		id   uint16
		name string
	}{
		{0x01, "Identity"},
		{0x02, "Message Router"},
		{0x03, "DeviceNet"},
		{0x04, "Assembly"},
		{0x05, "Connection"},
		{0x06, "Connection Manager"},
		{0x07, "Register"},
		{0x08, "Discrete Input Point"},
		{0x09, "Discrete Output Point"},
		{0x0A, "Analog Input Point"},
		{0x0B, "Analog Output Point"},
		{0x0F, "Parameter"},
		{0x10, "Parameter Group"},
		{0x1D, "Discrete Input Group"},
		{0x1E, "Discrete Output Group"},
		{0x1F, "Discrete Group"},
		{0x20, "Analog Input Group"},
		{0x21, "Analog Output Group"},
		{0x22, "Analog Group"},
		{0x28, "Motor Data"},
		{0x29, "Control Supervisor"},
		{0x2A, "AC/DC Drive"},
		{0x2B, "Acknowledge Handler"},
		{0x37, "S-Analog Sensor"},
		{0x38, "S-Analog Actuator"},
		{0x39, "S-Single Stage Controller"},
		{0x3A, "S-Gas Calibration"},
		{0x6B, "Symbol"},
		{0xAC, "IO (Logix)"},
		{0xB2, "Logix5000"},
		{0xF5, "TCP/IP Interface"},
		{0xF6, "Ethernet Link"},
	}

	for _, c := range classes {
		// GetAttributeAll on class-level (instance 0)
		resp, err := cipReq(cip.ServiceGetAttributeAll, cip.BuildPath(cip.UINT(c.id), 0, 0), nil)
		if err != nil || !resp.IsSuccess() {
			// Also try instance 1
			resp, err = cipReq(cip.ServiceGetAttributeAll, cip.BuildPath(cip.UINT(c.id), 1, 0), nil)
			if err != nil || !resp.IsSuccess() {
				continue
			}
		}
		fmt.Printf("  0x%02X  %-28s  (%d bytes)\n", c.id, c.name, len(resp.ResponseData))
	}

	// ---------------------------------------------------------------
	// Tags (Logix only, Class 0x6B)
	// ---------------------------------------------------------------

	if *probeTags {
		fmt.Println()
		fmt.Println("═══════════════════════════════════════════════════════════════")
		fmt.Println("  TAGS (LOGIX)")
		fmt.Println("═══════════════════════════════════════════════════════════════")

		// Try using the client's ListTags which queries Symbol Object class
		connCtx, connCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer connCancel()

		client, err := ethernetip.Connect(connCtx, tcpAddr)
		if err != nil {
			fmt.Printf("  (connect for tag list failed: %v)\n", err)
		} else {
			defer client.Close()

			tags, err := client.ListTags(connCtx)
			if err != nil {
				fmt.Printf("  (list tags failed: %v)\n", err)
			} else if len(tags) == 0 {
				fmt.Println("  (no tags found)")
			} else {
				fmt.Printf("  Found %d tags:\n", len(tags))
				show := len(tags)
				if show > 50 {
					show = 50
				}
				for i := 0; i < show; i++ {
					t := tags[i]
					typeName := cipTypeName(uint16(t.Type))
					fmt.Printf("    %-40s  type=0x%04X (%s)\n", t.Name, t.Type, typeName)
				}
				if len(tags) > show {
					fmt.Printf("    ... and %d more\n", len(tags)-show)
				}
			}
		}
	}

	// ---------------------------------------------------------------
	// Quick read of a few common tag names (Logix)
	// ---------------------------------------------------------------

	if *probeTags {
		fmt.Println()
		fmt.Println("═══════════════════════════════════════════════════════════════")
		fmt.Println("  SAMPLE TAG VALUES")
		fmt.Println("═══════════════════════════════════════════════════════════════")

		connCtx2, connCancel2 := context.WithTimeout(context.Background(), 10*time.Second)
		defer connCancel2()

		client2, err := ethernetip.Connect(connCtx2, tcpAddr)
		if err == nil {
			defer client2.Close()
			// Try to read any DINTs from the tag list
			tags, _ := client2.ListTags(connCtx2)
			count := 0
			for _, t := range tags {
				if count >= 10 {
					break
				}
				if t.Type == cip.TypeDINT || t.Type == cip.TypeREAL ||
					t.Type == cip.TypeINT || t.Type == cip.TypeBOOL {
					data, err := client2.ReadTag(connCtx2, t.Name)
					if err != nil {
						continue
					}
					valStr := formatTagValue(data, cip.DataType(t.Type))
					fmt.Printf("    %-40s = %s\n", t.Name, valStr)
					count++
				}
			}
			if count == 0 {
				fmt.Println("  (no readable scalar tags found)")
			}
		}
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("  PROBE COMPLETE")
	fmt.Println("═══════════════════════════════════════════════════════════════")
}

func vendorName(id uint16) string {
	switch id {
	case 1:
		return "Rockwell Automation"
	case 424:
		return "Teknic"
	default:
		return "Unknown"
	}
}

func deviceTypeName(dt uint16) string {
	switch dt {
	case 0:
		return "Generic"
	case 2:
		return "AC Drive"
	case 7:
		return "General Purpose Discrete I/O"
	case 12:
		return "Communications Adapter"
	case 14:
		return "Programmable Logic Controller"
	case 22:
		return "Safety Discrete I/O"
	case 24:
		return "HMI"
	case 33:
		return "Generic Safety"
	case 43:
		return "Servo Drive"
	case 150:
		return "AC/DC Drive (vendor specific)"
	default:
		return fmt.Sprintf("Type %d", dt)
	}
}

func cipTypeName(t uint16) string {
	if t&0x8000 != 0 {
		base := cipTypeName(t & 0x0FFF)
		return base + "[]"
	}
	switch cip.DataType(t) {
	case cip.TypeBOOL:
		return "BOOL"
	case cip.TypeSINT:
		return "SINT"
	case cip.TypeINT:
		return "INT"
	case cip.TypeDINT:
		return "DINT"
	case cip.TypeLINT:
		return "LINT"
	case cip.TypeREAL:
		return "REAL"
	case cip.TypeLREAL:
		return "LREAL"
	case cip.TypeSTRING:
		return "STRING"
	default:
		if t >= 0x0100 {
			return "STRUCT"
		}
		return fmt.Sprintf("0x%04X", t)
	}
}

func formatTagValue(data []byte, dt cip.DataType) string {
	if len(data) < 2 {
		return fmt.Sprintf("% X", data)
	}
	// Skip 2-byte type prefix
	hdrLen := 2
	typeCode := cip.DataType(binary.LittleEndian.Uint16(data[0:2]))
	if typeCode >= 0x02A0 {
		hdrLen = 4 // struct has 4-byte header
	}
	if len(data) < hdrLen {
		return fmt.Sprintf("% X", data)
	}
	payload := data[hdrLen:]

	switch dt {
	case cip.TypeBOOL:
		if len(payload) >= 1 {
			if payload[0] != 0 {
				return "true"
			}
			return "false"
		}
	case cip.TypeSINT:
		if len(payload) >= 1 {
			return fmt.Sprintf("%d", int8(payload[0]))
		}
	case cip.TypeINT:
		if len(payload) >= 2 {
			return fmt.Sprintf("%d", int16(binary.LittleEndian.Uint16(payload)))
		}
	case cip.TypeDINT:
		if len(payload) >= 4 {
			return fmt.Sprintf("%d", int32(binary.LittleEndian.Uint32(payload)))
		}
	case cip.TypeLINT:
		if len(payload) >= 8 {
			return fmt.Sprintf("%d", int64(binary.LittleEndian.Uint64(payload)))
		}
	case cip.TypeREAL:
		if len(payload) >= 4 {
			bits := binary.LittleEndian.Uint32(payload)
			return fmt.Sprintf("%.4f", math.Float32frombits(bits))
		}
	case cip.TypeLREAL:
		if len(payload) >= 8 {
			bits := binary.LittleEndian.Uint64(payload)
			return fmt.Sprintf("%.6f", math.Float64frombits(bits))
		}
	}
	return fmt.Sprintf("% X", data)
}
