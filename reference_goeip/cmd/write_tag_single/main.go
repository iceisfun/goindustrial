package main

import (
	"flag"
	"os"
	"strconv"

	"github.com/iceisfun/goeip/internal"
	"github.com/iceisfun/goeip/pkg/client"
)

func main() {
	address := flag.String("addr", "192.168.1.10:44818", "PLC Address (IP:Port)")
	tagName := flag.String("tag", "", "Tag Name to write (required)")
	valueStr := flag.String("value", "", "Value to write (required)")
	typeStr := flag.String("type", "", "Data type (BOOL, SINT, INT, DINT, LINT, USINT, UINT, UDINT, ULINT, REAL, LREAL, STRING) (required)")

	flag.Parse()

	if *tagName == "" || *valueStr == "" || *typeStr == "" {
		flag.Usage()
		os.Exit(1)
	}

	logger := internal.NewConsoleLogger()
	logger.Infof("Connecting to %s...", *address)

	// Parse value based on type
	var value any
	var err error

	switch *typeStr {
	case "BOOL":
		value, err = strconv.ParseBool(*valueStr)
	case "SINT":
		var v int64
		v, err = strconv.ParseInt(*valueStr, 10, 8)
		value = int8(v)
	case "INT":
		var v int64
		v, err = strconv.ParseInt(*valueStr, 10, 16)
		value = int16(v)
	case "DINT":
		var v int64
		v, err = strconv.ParseInt(*valueStr, 10, 32)
		value = int32(v)
	case "LINT":
		value, err = strconv.ParseInt(*valueStr, 10, 64)
	case "USINT":
		var v uint64
		v, err = strconv.ParseUint(*valueStr, 10, 8)
		value = uint8(v)
	case "UINT":
		var v uint64
		v, err = strconv.ParseUint(*valueStr, 10, 16)
		value = uint16(v)
	case "UDINT":
		var v uint64
		v, err = strconv.ParseUint(*valueStr, 10, 32)
		value = uint32(v)
	case "ULINT":
		value, err = strconv.ParseUint(*valueStr, 10, 64)
	case "REAL":
		var v float64
		v, err = strconv.ParseFloat(*valueStr, 32)
		value = float32(v)
	case "LREAL":
		value, err = strconv.ParseFloat(*valueStr, 64)
	case "STRING":
		value = *valueStr
	default:
		logger.Errorf("Unsupported type: %s", *typeStr)
		os.Exit(1)
	}

	if err != nil {
		logger.Errorf("Failed to parse value '%s' as %s: %v", *valueStr, *typeStr, err)
		os.Exit(1)
	}

	c, err := client.Connect(*address, logger)
	if err != nil {
		logger.Errorf("Failed to connect: %v", err)
		os.Exit(1)
	}
	defer c.Close()

	logger.Infof("Writing tag '%s' with value %v (%T)...", *tagName, value, value)
	err = c.WriteTag(*tagName, value)
	if err != nil {
		logger.Errorf("Failed to write tag: %v", err)
		os.Exit(1)
	}

	logger.Infof("Write success!")
}
