package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/iceisfun/goeip/internal"
	"github.com/iceisfun/goeip/pkg/cip"
	"github.com/iceisfun/goeip/pkg/client"
)

func main() {
	address := flag.String("addr", "192.168.1.10:44818", "PLC Address (IP:Port)")
	tagName := flag.String("tag", "TestTag", "Tag Name to read")
	tagType := flag.String("type", "int32", "Tag Type (int32, uint32, etc.)")
	flag.Parse()

	logger := internal.NopLogger()
	logger.Infof("Connecting to %s...", *address)

	c, err := client.Connect(*address, logger)
	if err != nil {
		logger.Errorf("Failed to connect: %v", err)
		os.Exit(1)
	}
	defer c.Close()

	logger.Infof("Monitoring tag '%s' (Type: %s) every 500ms...", *tagName, *tagType)

	var lastData []byte
	var lastChangeTime time.Time
	var initialized bool

	for {
		// Start of loop processing
		start := time.Now()

		data, err := c.ReadTag(*tagName)
		if time.Since(start) > 50*time.Millisecond {
			logger.Errorf("ReadTag took slow: %v", time.Since(start))
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "Read error: %v\n", err)
		} else {
			if !initialized || !bytes.Equal(data, lastData) {
				now := time.Now()
				durationStr := "N/A"
				label := "[INITIAL]"

				if initialized {
					label = "[CHANGE]"
					if !lastChangeTime.IsZero() {
						durationStr = now.Sub(lastChangeTime).String()
					}
				}

				valStr := decodeData(data, *tagType)
				fmt.Printf("%s %s Value: %s | Since last: %s\n", time.Now().Format("2006/01/02 15:04:05"), label, valStr, durationStr)

				lastData = data
				lastChangeTime = now
				initialized = true
			}
		}

		// Sleep for remaining time to match 500ms interval, or just fixed delay?
		// "loop with a 500ms delay" usually implies sleep 500ms.
		time.Sleep(500 * time.Millisecond)
		_ = start // unused
	}
}

func decodeData(data []byte, typeName string) string {
	buf := bytes.NewReader(data)
	switch typeName {
	case "bool":
		if len(data) > 0 {
			return fmt.Sprintf("%v", data[0] != 0)
		}
	case "int8":
		var v int8
		binary.Read(buf, binary.LittleEndian, &v)
		return fmt.Sprintf("%d", v)
	case "uint8":
		var v uint8
		binary.Read(buf, binary.LittleEndian, &v)
		return fmt.Sprintf("%d", v)
	case "int16":
		var v int16
		binary.Read(buf, binary.LittleEndian, &v)
		return fmt.Sprintf("%d", v)
	case "uint16":
		var v uint16
		binary.Read(buf, binary.LittleEndian, &v)
		return fmt.Sprintf("%d", v)
	case "int32", "DINT":
		var v int32
		binary.Read(buf, binary.LittleEndian, &v)
		return fmt.Sprintf("%d", v)
	case "uint32", "UDINT":
		var v uint32
		binary.Read(buf, binary.LittleEndian, &v)
		return fmt.Sprintf("%d", v)
	case "int64":
		var v int64
		binary.Read(buf, binary.LittleEndian, &v)
		return fmt.Sprintf("%d", v)
	case "uint64":
		var v uint64
		binary.Read(buf, binary.LittleEndian, &v)
		return fmt.Sprintf("%d", v)
	case "float32", "REAL":
		var v float32
		binary.Read(buf, binary.LittleEndian, &v)
		return fmt.Sprintf("%f", v)
	case "float64", "LREAL":
		var v float64
		binary.Read(buf, binary.LittleEndian, &v)
		return fmt.Sprintf("%f", v)
	case "timer":
		// We can try to decode as struct if we had the struct definition handy here,
		// relying on manual decoding similar to read_tag_struct if needed.
		// For now, raw hex if not simple type?
		// Let's try to replicate the timer decoding from read_tag_struct if simple.
		// read_tag_struct uses cip.Timer.
		// We can't easily use cip.Timer with binary.Read directly if it has padding/tags,
		// but let's assume standard packing.
		if len(data) >= 12 {
			t, err := cip.DecodeTimer(data)
			if err == nil {
				return fmt.Sprintf("TIMER{PRE=%d, ACC=%d, EN=%v, TT=%v, DN=%v}", t.PRE, t.ACC, t.EN, t.TT, t.DN)
			}
		}
	case "counter", "COUNTER":
		if len(data) >= 12 {
			c, err := cip.DecodeCounter(data)
			if err == nil {
				return fmt.Sprintf("COUNTER{PRE=%d, ACC=%d, CU=%v, CD=%v, DN=%v, OV=%v, UN=%v}", c.PRE, c.ACC, c.CU, c.CD, c.DN, c.OV, c.UN)
			}
		}
	}
	// Default/Fallback
	return fmt.Sprintf("RAW[% X]", data)
}
