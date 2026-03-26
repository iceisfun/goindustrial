package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/iceisfun/goeip/pkg/cip"
	"github.com/iceisfun/goeip/pkg/objects/assembly"
	"github.com/iceisfun/goeip/pkg/objects/connmgr"
	"github.com/iceisfun/goeip/pkg/runtime"
	"github.com/iceisfun/goeip/pkg/server"
)

func main() {
	var (
		addr           = flag.String("addr", ":44818", "TCP address to listen on")
		udpAddr        = flag.String("udp-addr", ":2222", "UDP address to listen on")
		inputAssembly  = flag.String("input-assembly", "", "Input Assembly ID=File (e.g. 100=data/in.bin)")
		outputAssembly = flag.String("output-assembly", "", "Output Assembly ID=File (e.g. 150=data/out.bin)")
	)
	flag.Parse()

	// 1. Initialize Objects
	ao := assembly.NewAssemblyObject()
	cm := connmgr.NewConnectionManager()

	// Register Assemblies
	if *inputAssembly != "" {
		parts := strings.Split(*inputAssembly, "=")
		id, _ := strconv.Atoi(parts[0])
		// Load file or create dummy data
		data := make([]byte, 32) // Default 32 bytes
		if len(parts) > 1 {
			// Load file logic here if needed, for now just use dummy
			log.Printf("Registered Input Assembly %d", id)
		}
		ao.RegisterAssembly(uint32(id), data)
	}
	if *outputAssembly != "" {
		parts := strings.Split(*outputAssembly, "=")
		id, _ := strconv.Atoi(parts[0])
		data := make([]byte, 32)
		log.Printf("Registered Output Assembly %d", id)
		ao.RegisterAssembly(uint32(id), data)
	}

	// 2. Initialize Router
	router := cip.NewMessageRouter()
	router.RegisterObject(cip.ClassAssembly, ao)
	router.RegisterObject(cip.ClassConnectionMgr, cm)

	// 3. Initialize Runtime (UDP)
	rt := runtime.NewRuntime(ao)

	// 4. Initialize Server (TCP)
	srv := server.NewServer(router)

	// Start UDP Runtime
	if err := rt.Start(*udpAddr); err != nil {
		log.Fatalf("Failed to start UDP runtime: %v", err)
	}
	log.Printf("UDP Runtime listening on %s", *udpAddr)

	// Start TCP Server
	if err := srv.Start(*addr); err != nil {
		log.Fatalf("Failed to start TCP server: %v", err)
	}
	log.Printf("TCP Server listening on %s", *addr)

	// Wait for signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
}
