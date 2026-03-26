package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/iceisfun/goeip/internal"
	"github.com/iceisfun/goeip/pkg/cip"
	"github.com/iceisfun/goeip/pkg/client"
)

func main() {
	address := flag.String("addr", "192.168.1.10:44818", "PLC Address (IP:Port)")
	tagName := flag.String("tag", "TestTag", "Tag Name to read")
	tagType := flag.String("type", "int32", "Tag Type to read")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nValid types:\n")
		fmt.Fprintf(os.Stderr, "  bool, int8, uint8, int16, uint16, int32, uint32, int64, uint64, float32, float64, timer, counter, custom\n")
	}

	flag.Parse()

	logger := internal.NewConsoleLogger()
	logger.Infof("Connecting to %s...", *address)

	c, err := client.Connect(*address, logger)
	if err != nil {
		logger.Errorf("Failed to connect: %v", err)
		os.Exit(1)
	}
	defer c.Close()

	logger.Infof("Reading tag '%s' as %s...", *tagName, *tagType)

	var readErr error
	switch strings.ToLower(*tagType) {
	case "bool":
		var val bool
		readErr = c.ReadTagInto(*tagName, &val)
		if readErr == nil {
			logger.Infof("Value: %v", val)
		}
	case "int8":
		var val int8
		readErr = c.ReadTagInto(*tagName, &val)
		if readErr == nil {
			logger.Infof("Value: %d", val)
		}
	case "uint8":
		var val uint8
		readErr = c.ReadTagInto(*tagName, &val)
		if readErr == nil {
			logger.Infof("Value: %d", val)
		}
	case "int16":
		var val int16
		readErr = c.ReadTagInto(*tagName, &val)
		if readErr == nil {
			logger.Infof("Value: %d", val)
		}
	case "uint16":
		var val uint16
		readErr = c.ReadTagInto(*tagName, &val)
		if readErr == nil {
			logger.Infof("Value: %d", val)
		}
	case "int32":
		var val int32
		readErr = c.ReadTagInto(*tagName, &val)
		if readErr == nil {
			logger.Infof("Value: %d", val)
		}
	case "uint32":
		var val uint32
		readErr = c.ReadTagInto(*tagName, &val)
		if readErr == nil {
			logger.Infof("Value: %d", val)
		}
	case "int64":
		var val int64
		readErr = c.ReadTagInto(*tagName, &val)
		if readErr == nil {
			logger.Infof("Value: %d", val)
		}
	case "uint64":
		var val uint64
		readErr = c.ReadTagInto(*tagName, &val)
		if readErr == nil {
			logger.Infof("Value: %d", val)
		}
	case "float32":
		var val float32
		readErr = c.ReadTagInto(*tagName, &val)
		if readErr == nil {
			logger.Infof("Value: %f", val)
		}
	case "float64":
		var val float64
		readErr = c.ReadTagInto(*tagName, &val)
		if readErr == nil {
			logger.Infof("Value: %f", val)
		}
	case "timer":
		var t cip.Timer
		readErr = c.ReadTagInto(*tagName, &t)
		if readErr == nil {
			logger.Infof("Timer: PRE=%d, ACC=%d, EN=%v, TT=%v, DN=%v", t.PRE, t.ACC, t.EN, t.TT, t.DN)
		}
	case "counter":
		var cnt cip.Counter
		readErr = c.ReadTagInto(*tagName, &cnt)
		if readErr == nil {
			logger.Infof("Counter: PRE=%d, ACC=%d, CU=%v, CD=%v, DN=%v, OV=%v, UN=%v", cnt.PRE, cnt.ACC, cnt.CU, cnt.CD, cnt.DN, cnt.OV, cnt.UN)
		}
	case "custom":
		var cst CustomStruct
		readErr = c.ReadTagInto(*tagName, &cst)
		if readErr == nil {
			logger.Infof("CustomStruct: Field1=%d, Field2=%f", cst.Field1, cst.Field2)
		}
	default:
		logger.Errorf("Unknown type: %s", *tagType)
		flag.Usage()
		os.Exit(1)
	}

	if readErr != nil {
		logger.Errorf("Failed to read tag: %v", readErr)
		os.Exit(1)
	}
}

// CustomStruct is an example of a user-defined struct that implements Unmarshaler.
type CustomStruct struct {
	Field1 int32
	Field2 float32
}

// UnmarshalCIP implements cip.Unmarshaler
func (cs *CustomStruct) UnmarshalCIP(data []byte) error {
	if len(data) < 8 {
		return fmt.Errorf("insufficient data for CustomStruct")
	}
	// Simple manual decoding for demonstration
	// In reality, you might use binary.Read or custom logic
	buf := bytes.NewReader(data)
	if err := binary.Read(buf, binary.LittleEndian, &cs.Field1); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.LittleEndian, &cs.Field2); err != nil {
		return err
	}
	return nil
}
