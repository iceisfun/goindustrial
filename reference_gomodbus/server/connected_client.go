package server

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Moonlight-Companies/gomodbus/common"
)

// clientConn is the internal per-connection tracking state.
// It contains atomics and a net.Conn, so it must not be copied.
type clientConn struct {
	remoteAddr  string
	connectedAt time.Time
	conn        net.Conn
	rxCount     atomic.Uint64
	txCount     atomic.Uint64
	fcCount     [256]atomic.Uint64
}

// ConnectedClient is a snapshot of a connected client's state.
// Returned by TCPServer.ConnectedClients(). Safe to copy and store.
type ConnectedClient struct {
	// RemoteAddr is the remote address of the connected client.
	RemoteAddr string

	// ConnectedAt is the time the client connected.
	ConnectedAt time.Time

	// RxTransactions is the number of requests received from this client.
	RxTransactions uint64

	// TxTransactions is the number of responses sent to this client.
	TxTransactions uint64

	// FunctionCodeStats is a per-function-code count of received requests.
	// Only non-zero entries are included.
	FunctionCodeStats map[common.FunctionCode]uint64
}

// String returns a human-readable summary of the connected client.
func (c ConnectedClient) String() string {
	duration := time.Since(c.ConnectedAt).Truncate(time.Second)
	s := fmt.Sprintf("%s | connected %s | rx: %d tx: %d", c.RemoteAddr, duration, c.RxTransactions, c.TxTransactions)
	if len(c.FunctionCodeStats) > 0 {
		// Sort by function code for deterministic output
		codes := make([]common.FunctionCode, 0, len(c.FunctionCodeStats))
		for fc := range c.FunctionCodeStats {
			codes = append(codes, fc)
		}
		sort.Slice(codes, func(i, j int) bool { return codes[i] < codes[j] })

		parts := make([]string, 0, len(codes))
		for _, fc := range codes {
			parts = append(parts, fmt.Sprintf("%s=%d", fc, c.FunctionCodeStats[fc]))
		}
		s += " | fc: " + strings.Join(parts, " ")
	}
	return s
}

// fcSnapshot creates a FunctionCodeStats map from a clientConn's atomic counters.
// Only non-zero entries are included.
func fcSnapshot(c *clientConn) map[common.FunctionCode]uint64 {
	stats := make(map[common.FunctionCode]uint64)
	for i := range c.fcCount {
		if v := c.fcCount[i].Load(); v > 0 {
			stats[common.FunctionCode(i)] = v
		}
	}
	return stats
}
