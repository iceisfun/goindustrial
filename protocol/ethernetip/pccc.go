package ethernetip

import (
	"context"
	"fmt"

	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
)

// Default values used in the PCCC requestor-ID header (the CIP Vendor ID +
// Serial Number block that prefixes every Execute_PCCC request). These are
// the originator stamp; an SLC/MicroLogix controller does not validate
// them, so any caller value is acceptable. Override with [WithCIPVendorID]
// and [WithCIPSerialNumber] when a deployment policy mandates specific IDs.
const (
	// DefaultCIPVendorID is the originator's CIP vendor ID written into
	// the Execute_PCCC requestor header. Anthropic-style 0x1234 marker.
	DefaultCIPVendorID uint16 = 0x1234

	// DefaultCIPSerialNumber is the originator's CIP serial number
	// written into the Execute_PCCC requestor header.
	DefaultCIPSerialNumber uint32 = 0x12345678
)

// WithCIPVendorID overrides the CIP vendor ID written into the requestor-ID
// header of every Execute_PCCC (0x4B) request issued by the client.
func WithCIPVendorID(id uint16) ClientOption {
	return func(c *Client) { c.cipVendorID = id }
}

// WithCIPSerialNumber overrides the CIP serial number written into the
// requestor-ID header of every Execute_PCCC (0x4B) request.
func WithCIPSerialNumber(sn uint32) ClientOption {
	return func(c *Client) { c.cipSerialNumber = sn }
}

// ExecutePCCC sends a raw PCCC command via the CIP Execute_PCCC service
// (0x4B) addressed to the PCCC Object (class 0x67, instance 1) and returns
// the raw PCCC reply payload. This is the low-level escape hatch — most
// callers should use the higher-level pccc package instead.
//
// The PCCC bytes are typically produced by pccc.EncodeTypedRead or
// pccc.EncodeTypedWrite; the returned bytes are suitable for
// pccc.DecodeReply.
//
// Errors fall into three categories:
//   - Transport errors trigger reconnect + retry per the client config.
//   - CIP-level errors (non-zero general status from the PCCC Object) are
//     returned as [cip.Error] and are NOT retried.
//   - The PCCC reply may itself contain a non-zero STS — that is signaled
//     by pccc.DecodeReply via [pccc.Error], not by this method.
func (c *Client) ExecutePCCC(ctx context.Context, pcccCmd []byte) ([]byte, error) {
	if len(pcccCmd) == 0 {
		return nil, fmt.Errorf("ethernetip: ExecutePCCC: empty PCCC command")
	}

	req := cip.NewExecutePCCCRequest(
		cip.UINT(c.cipVendorID),
		cip.UDINT(c.cipSerialNumber),
		pcccCmd,
	)

	var reply []byte
	err := c.do(ctx, func(sess *Session) error {
		resp, err := sess.SendCIPRequest(ctx, req)
		if err != nil {
			return err
		}
		if err := resp.Error(); err != nil {
			return &cipError{err}
		}
		// Strip the echoed 7-byte requestor ID. What remains is the raw
		// PCCC reply that the device produced.
		if len(resp.ResponseData) < 7 {
			return fmt.Errorf("ethernetip: Execute_PCCC reply too short (%d bytes, need >=7 for requestor ID)",
				len(resp.ResponseData))
		}
		reply = append([]byte(nil), resp.ResponseData[7:]...)
		return nil
	})
	return reply, err
}
