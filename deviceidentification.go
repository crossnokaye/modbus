// Copyright 2014 Quoc-Viet Nguyen. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD license. See the LICENSE file for details.

package modbus

import (
	"fmt"
	"sort"
	"strings"
)

const (
	// FuncCodeReadDeviceIdentification is the Encapsulated Interface
	// Transport function code used for Read Device Identification.
	FuncCodeReadDeviceIdentification = 0x2B

	// MEITypeReadDeviceIdentification is the MEI (Modbus Encapsulated
	// Interface) type carried by a Read Device Identification
	// request/response.
	MEITypeReadDeviceIdentification = 0x0E
)

const (
	// Read Device ID codes select which set of identification objects to
	// read. Codes 1-3 use stream access (potentially several messages);
	// code 4 reads a single object by id (individual access).
	ReadDeviceIDCodeBasic      = 1 // mandatory objects 0x00-0x02
	ReadDeviceIDCodeRegular    = 2 // optional objects 0x03-0x7F
	ReadDeviceIDCodeExtended   = 3 // extended objects 0x80-0xFF
	ReadDeviceIDCodeIndividual = 4 // single object specified by object id
)

const (
	// Well-known Read Device Identification object ids. Objects 0x00-0x02
	// are mandatory (basic); the rest are optional. Values are
	// vendor-defined byte strings.
	DeviceIDObjectVendorName          = 0x00
	DeviceIDObjectProductCode         = 0x01
	DeviceIDObjectMajorMinorRevision  = 0x02
	DeviceIDObjectVendorURL           = 0x03
	DeviceIDObjectProductName         = 0x04
	DeviceIDObjectModelName           = 0x05
	DeviceIDObjectUserApplicationName = 0x06
)

// DeviceIDObjectName returns the Modbus-standard name for a well-known
// Read Device Identification object id (0x00-0x06), or "" if the id has no
// standardized name (reserved 0x07-0x7F or vendor-private 0x80-0xFF). The
// mapping is defined by the Modbus specification and is the same for every
// device, so callers should rely on this rather than maintaining their own.
func DeviceIDObjectName(id byte) string {
	switch id {
	case DeviceIDObjectVendorName:
		return "VendorName"
	case DeviceIDObjectProductCode:
		return "ProductCode"
	case DeviceIDObjectMajorMinorRevision:
		return "MajorMinorRevision"
	case DeviceIDObjectVendorURL:
		return "VendorUrl"
	case DeviceIDObjectProductName:
		return "ProductName"
	case DeviceIDObjectModelName:
		return "ModelName"
	case DeviceIDObjectUserApplicationName:
		return "UserApplicationName"
	default:
		return ""
	}
}

// DeviceIdentificationClient extends Client with the Read Device
// Identification function. It is kept separate from Client so that adding
// device identification does not break existing types that implement (or
// mock) the Client interface. Obtain one with NewDeviceIdentificationClient,
// or type-assert a Client returned by NewClient/NewClient2/TCPClient:
//
//	if dic, ok := client.(modbus.DeviceIdentificationClient); ok {
//		id, err := dic.ReadDeviceIdentification(modbus.ReadDeviceIDCodeBasic, 0)
//	}
type DeviceIdentificationClient interface {
	Client

	// ReadDeviceIdentification reads identification objects from a remote
	// device using the Read Device Identification function (function code
	// 0x2B, MEI type 0x0E). readDeviceIDCode selects the access type:
	// 1 (basic), 2 (regular) and 3 (extended) use stream access and the
	// returned objects are gathered across as many messages as the device
	// reports via "More Follows"; 4 (individual) reads the single object
	// identified by objectID. For stream access objectID is the starting
	// object id (use 0 to start from the beginning). Modbus exception
	// responses are returned as *ModbusError.
	ReadDeviceIdentification(readDeviceIDCode, objectID byte) (result DeviceIdentification, err error)
}

// NewDeviceIdentificationClient creates a client that supports the full
// Client API plus Read Device Identification, using the given backend handler.
func NewDeviceIdentificationClient(handler ClientHandler) DeviceIdentificationClient {
	return &client{packager: handler, transporter: handler}
}

// maxDeviceIDMessages bounds the number of messages read while following a
// "More Follows" stream, guarding against a device that never completes the
// transfer or fails to advance the next object id.
const maxDeviceIDMessages = 256

// DeviceIdentification holds the identification objects returned by a
// Read Device Identification request.
type DeviceIdentification struct {
	// ConformityLevel is the conformity level reported by the device
	// (e.g. 0x01/0x81 basic, 0x02/0x82 regular, 0x03/0x83 extended).
	ConformityLevel byte
	// Objects maps each returned object id to its raw value bytes. Values
	// are vendor-defined; well-known object ids are defined as
	// DeviceIDObject* constants.
	Objects map[byte][]byte
}

// SortedIDs returns the ids of the returned objects in ascending order, for
// deterministic iteration and display (the Objects map itself has no order).
func (d DeviceIdentification) SortedIDs() []byte {
	ids := make([]byte, 0, len(d.Objects))
	for id := range d.Objects {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// String renders the identification objects in a human-readable form,
// sorted by object id. Printable ASCII values are shown as text; other
// values fall back to a hex dump.
func (d DeviceIdentification) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "conformity level: 0x%02X", d.ConformityLevel)
	for _, id := range d.SortedIDs() {
		fmt.Fprintf(&b, "\nobject 0x%02X: %s", id, formatObjectValue(d.Objects[id]))
	}
	return b.String()
}

// formatObjectValue renders an object value as text when it is printable
// ASCII, otherwise as a space-separated hex dump.
func formatObjectValue(value []byte) string {
	for _, c := range value {
		if c < 0x20 || c > 0x7E {
			return fmt.Sprintf("% x", value)
		}
	}
	return string(value)
}

// Request:
//  Function code         : 1 byte (0x2B)
//  MEI type              : 1 byte (0x0E)
//  Read Device ID code   : 1 byte (0x01-0x04)
//  Object Id             : 1 byte
// Response:
//  Function code         : 1 byte (0x2B)
//  MEI type              : 1 byte (0x0E)
//  Read Device ID code   : 1 byte
//  Conformity level      : 1 byte
//  More Follows          : 1 byte (0x00 or 0xFF)
//  Next Object Id        : 1 byte
//  Number of objects     : 1 byte
//  Object list           : N* (Object Id 1 byte, Object length 1 byte, Object value)
func (mb *client) ReadDeviceIdentification(readDeviceIDCode, objectID byte) (result DeviceIdentification, err error) {
	if readDeviceIDCode < ReadDeviceIDCodeBasic || readDeviceIDCode > ReadDeviceIDCodeIndividual {
		err = fmt.Errorf("modbus: read device id code '%v' must be between '%v' and '%v'", readDeviceIDCode, ReadDeviceIDCodeBasic, ReadDeviceIDCodeIndividual)
		return
	}
	result.Objects = make(map[byte][]byte)

	nextObjectID := objectID
	for i := 0; ; i++ {
		if i >= maxDeviceIDMessages {
			err = fmt.Errorf("modbus: read device identification did not complete after '%v' messages", maxDeviceIDMessages)
			return
		}
		request := ProtocolDataUnit{
			FunctionCode: FuncCodeReadDeviceIdentification,
			Data:         []byte{MEITypeReadDeviceIdentification, readDeviceIDCode, nextObjectID},
		}
		var response *ProtocolDataUnit
		response, err = mb.send(&request)
		if err != nil {
			return
		}
		var moreFollows bool
		var next byte
		moreFollows, next, err = parseDeviceIdentification(response.Data, &result)
		if err != nil {
			return
		}
		// Individual access (code 4) returns a single object and never
		// continues, regardless of the More Follows flag.
		if readDeviceIDCode == ReadDeviceIDCodeIndividual || !moreFollows {
			return
		}
		// Guard against a device that signals More Follows but fails to
		// advance the next object id, which would loop forever.
		if next == nextObjectID {
			err = fmt.Errorf("modbus: read device identification next object id '%v' did not advance", next)
			return
		}
		nextObjectID = next
	}
}

// parseDeviceIdentification decodes a Read Device Identification response
// body (starting at the MEI type byte), merges the returned objects into
// result, and reports whether more objects follow along with the next
// object id to request.
func parseDeviceIdentification(data []byte, result *DeviceIdentification) (moreFollows bool, nextObjectID byte, err error) {
	// MEI type, read device id code, conformity level, more follows,
	// next object id, number of objects.
	const headerLen = 6
	if len(data) < headerLen {
		err = fmt.Errorf("modbus: response data size '%v' is less than expected '%v'", len(data), headerLen)
		return
	}
	if data[0] != MEITypeReadDeviceIdentification {
		err = fmt.Errorf("modbus: response MEI type '%v' does not match expected '%v'", data[0], MEITypeReadDeviceIdentification)
		return
	}
	result.ConformityLevel = data[2]
	moreFollows = data[3] == 0xFF
	nextObjectID = data[4]
	numObjects := int(data[5])

	offset := headerLen
	for i := 0; i < numObjects; i++ {
		// Each object is at least an id byte and a length byte.
		if offset+2 > len(data) {
			err = fmt.Errorf("modbus: response truncated reading object '%v' of '%v'", i+1, numObjects)
			return
		}
		id := data[offset]
		length := int(data[offset+1])
		offset += 2
		if offset+length > len(data) {
			err = fmt.Errorf("modbus: response object '%v' length '%v' exceeds available data", id, length)
			return
		}
		value := make([]byte, length)
		copy(value, data[offset:offset+length])
		result.Objects[id] = value
		offset += length
	}
	if offset != len(data) {
		err = fmt.Errorf("modbus: response data size '%v' does not match object count '%v'", len(data), numObjects)
		return
	}
	return
}
