package client

import (
	"fmt"

	"github.com/iceisfun/goeip/pkg/cip"
	"github.com/iceisfun/goeip/pkg/eip"
	"github.com/iceisfun/goeip/pkg/session"
)

// ListIdentity lists the identity of the target.
func (c *Client) ListIdentity() ([]eip.ListIdentityItem, error) {
	var result []eip.ListIdentityItem
	err := c.do(func(sess *session.Session) error {
		items, err := sess.ListIdentity()
		if err != nil {
			return err
		}
		result = items
		return nil
	})
	return result, err
}

// ListServices lists the services supported by the target.
func (c *Client) ListServices() ([]eip.ListServicesItem, error) {
	var result []eip.ListServicesItem
	err := c.do(func(sess *session.Session) error {
		items, err := sess.ListServices()
		if err != nil {
			return err
		}
		result = items
		return nil
	})
	return result, err
}

// ListTags lists all tags on the PLC by iterating the Symbol Object.
// The entire enumeration runs inside a single do() call so a mid-enumeration
// connection drop retries from scratch.
func (c *Client) ListTags() ([]cip.SymbolInstance, error) {
	var result []cip.SymbolInstance
	err := c.do(func(sess *session.Session) error {
		reqClass := cip.NewGetSymbolClassAttributesRequest()
		respClass, err := sess.SendCIPRequest(reqClass)
		if err != nil {
			return err
		}
		if !respClass.IsSuccess() {
			return &cipError{respClass.Error()}
		}

		_, maxInstance, err := cip.DecodeSymbolClassAttributesResponse(respClass.ResponseData)
		if err != nil {
			return fmt.Errorf("failed to decode symbol class attributes: %w", err)
		}

		c.logger.Infof("Max Symbol Instance: %d", maxInstance)

		var allSymbols []cip.SymbolInstance

		for id := uint32(1); id <= uint32(maxInstance); id++ {
			req := cip.NewGetSymbolAttributesRequest(id)
			resp, err := sess.SendCIPRequest(req)
			if err != nil {
				c.logger.Warnf("Failed to fetch attributes for instance %d: %v", id, err)
				continue
			}

			if !resp.IsSuccess() {
				if resp.GeneralStatus == cip.StatusObjectDoesNotExist || resp.GeneralStatus == cip.StatusPathDestinationUnknown {
					continue
				}
				continue
			}

			name, typeCode, err := cip.DecodeSymbolAttributesResponse(resp.ResponseData)
			if err != nil {
				c.logger.Warnf("Failed to decode attributes for instance %d: %v", id, err)
				continue
			}

			if name != "" {
				allSymbols = append(allSymbols, cip.SymbolInstance{
					InstanceID: id,
					Name:       name,
					Type:       typeCode,
				})
			}
		}

		result = allSymbols
		return nil
	})
	return result, err
}
