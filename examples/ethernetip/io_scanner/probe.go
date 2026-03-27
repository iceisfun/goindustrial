//go:build ignore

package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"os"

	"github.com/iceisfun/goindustrial/protocol/ethernetip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: go run probe.go <PLC_IP>")
		os.Exit(1)
	}
	addr := os.Args[1]

	tc, err := ethernetip.NewTCPConn(net.JoinHostPort(addr, "44818"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connect to %s: %v\n", addr, err)
		os.Exit(1)
	}
	sess := ethernetip.NewSession(tc, nil)
	ctx := context.Background()
	if err := sess.Register(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Register session: %v\n", err)
		tc.Close()
		os.Exit(1)
	}
	defer func() { sess.Unregister(ctx); tc.Close() }()

	// Identity
	resp, err := sess.SendCIPRequest(ctx, &cip.MessageRouterRequest{
		Service:     cip.ServiceGetAttributeAll,
		RequestPath: cip.BuildPath(cip.ClassIdentity, 1, 0),
	})
	if err != nil {
		fmt.Println("Identity error:", err)
	} else if resp.IsSuccess() && len(resp.ResponseData) >= 15 {
		d := resp.ResponseData
		vendor := binary.LittleEndian.Uint16(d[0:2])
		devType := binary.LittleEndian.Uint16(d[2:4])
		prodCode := binary.LittleEndian.Uint16(d[4:6])
		revMajor := d[6]
		revMinor := d[7]
		serial := binary.LittleEndian.Uint32(d[10:14])
		nameLen := d[14]
		name := ""
		if int(15+nameLen) <= len(d) {
			name = string(d[15 : 15+nameLen])
		}
		fmt.Printf("Device: %s\n", name)
		fmt.Printf("  Vendor ID: %d, Device Type: %d, Product Code: %d\n", vendor, devType, prodCode)
		fmt.Printf("  Revision: %d.%d, Serial: 0x%08X\n", revMajor, revMinor, serial)
	} else {
		fmt.Printf("Identity read failed: status 0x%02X\n", resp.GeneralStatus)
	}

	// Probe assembly instances
	fmt.Println("\nAssembly instances (Class 0x04, Attribute 3 = data):")
	found := 0
	for inst := uint16(1); inst <= 255; inst++ {
		path := cip.BuildPath(cip.ClassAssembly, cip.UINT(inst), 3)
		resp, err := sess.SendCIPRequest(ctx, &cip.MessageRouterRequest{
			Service:     cip.ServiceGetAttributeSingle,
			RequestPath: path,
		})
		if err != nil {
			continue
		}
		if resp.IsSuccess() {
			found++
			data := resp.ResponseData
			preview := data
			if len(preview) > 32 {
				preview = preview[:32]
			}
			fmt.Printf("  Instance %3d: %3d bytes  % X", inst, len(data), preview)
			if len(data) > 32 {
				fmt.Printf("...")
			}
			fmt.Println()
		}
	}
	if found == 0 {
		fmt.Println("  (none found)")
	}

	// Also probe TCP/IP interface for IP info
	fmt.Println("\nTCP/IP Interface (Class 0xF5, Instance 1):")
	resp, err = sess.SendCIPRequest(ctx, &cip.MessageRouterRequest{
		Service:     cip.ServiceGetAttributeAll,
		RequestPath: cip.BuildPath(0xF5, 1, 0),
	})
	if err == nil && resp.IsSuccess() {
		fmt.Printf("  %d bytes: % X\n", len(resp.ResponseData), resp.ResponseData)
	}
}
