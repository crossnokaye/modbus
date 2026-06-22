// Copyright 2014 Quoc-Viet Nguyen. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD license. See the LICENSE file for details.

package modbus

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
)

// mockHandler implements ClientHandler (Packager + Transporter) with a
// trivial framing (adu = function code followed by PDU data) so device
// identification responses can be driven without a live device.
type mockHandler struct {
	requests  [][]byte
	responses [][]byte
	idx       int
	// gen, when set, generates a response from the request index and the
	// request adu, overriding responses. Used to drive unbounded streams.
	gen func(reqIndex int, req []byte) []byte
}

func (m *mockHandler) Encode(pdu *ProtocolDataUnit) ([]byte, error) {
	adu := make([]byte, 1+len(pdu.Data))
	adu[0] = pdu.FunctionCode
	copy(adu[1:], pdu.Data)
	return adu, nil
}

func (m *mockHandler) Decode(adu []byte) (*ProtocolDataUnit, error) {
	return &ProtocolDataUnit{FunctionCode: adu[0], Data: adu[1:]}, nil
}

func (m *mockHandler) Verify(aduRequest, aduResponse []byte) error { return nil }

func (m *mockHandler) Send(aduRequest []byte) ([]byte, error) {
	i := len(m.requests)
	m.requests = append(m.requests, aduRequest)
	if m.gen != nil {
		return m.gen(i, aduRequest), nil
	}
	if m.idx >= len(m.responses) {
		return nil, fmt.Errorf("mock: no queued response for request %d", i)
	}
	resp := m.responses[m.idx]
	m.idx++
	return resp, nil
}

type idObject struct {
	id    byte
	value []byte
}

// deviceIDResponse builds a Read Device Identification response adu.
func deviceIDResponse(code, conformity, moreFollows, nextID byte, objects []idObject) []byte {
	adu := []byte{
		FuncCodeReadDeviceIdentification,
		MEITypeReadDeviceIdentification,
		code,
		conformity,
		moreFollows,
		nextID,
		byte(len(objects)),
	}
	for _, o := range objects {
		adu = append(adu, o.id, byte(len(o.value)))
		adu = append(adu, o.value...)
	}
	return adu
}

func TestReadDeviceIdentificationBasic(t *testing.T) {
	h := &mockHandler{responses: [][]byte{
		deviceIDResponse(ReadDeviceIDCodeBasic, 0x81, 0x00, 0x00, []idObject{
			{DeviceIDObjectVendorName, []byte("Acme")},
			{DeviceIDObjectProductCode, []byte("PC-1")},
			{DeviceIDObjectMajorMinorRevision, []byte("1.2")},
		}),
	}}
	client := NewDeviceIdentificationClient(h)

	result, err := client.ReadDeviceIdentification(ReadDeviceIDCodeBasic, 0x00)
	if err != nil {
		t.Fatal(err)
	}
	if result.ConformityLevel != 0x81 {
		t.Fatalf("conformity level: expected 0x81, actual 0x%02X", result.ConformityLevel)
	}
	if len(result.Objects) != 3 {
		t.Fatalf("objects: expected 3, actual %d", len(result.Objects))
	}
	if got := string(result.Objects[DeviceIDObjectVendorName]); got != "Acme" {
		t.Fatalf("vendor name: expected Acme, actual %q", got)
	}
	if got := string(result.Objects[DeviceIDObjectMajorMinorRevision]); got != "1.2" {
		t.Fatalf("revision: expected 1.2, actual %q", got)
	}
	// Request must be [0x2B, 0x0E, code, objectID].
	expectedReq := []byte{FuncCodeReadDeviceIdentification, MEITypeReadDeviceIdentification, ReadDeviceIDCodeBasic, 0x00}
	if len(h.requests) != 1 || !bytes.Equal(h.requests[0], expectedReq) {
		t.Fatalf("request: expected %x, actual %x", expectedReq, h.requests)
	}
}

func TestReadDeviceIdentificationStreamMoreFollows(t *testing.T) {
	h := &mockHandler{responses: [][]byte{
		// First message: more follows, continue from object 0x02.
		deviceIDResponse(ReadDeviceIDCodeRegular, 0x82, 0xFF, 0x02, []idObject{
			{DeviceIDObjectVendorName, []byte("Acme")},
			{DeviceIDObjectProductCode, []byte("PC-1")},
		}),
		// Second message: completes the stream.
		deviceIDResponse(ReadDeviceIDCodeRegular, 0x82, 0x00, 0x00, []idObject{
			{DeviceIDObjectMajorMinorRevision, []byte("1.2")},
		}),
	}}
	client := NewDeviceIdentificationClient(h)

	result, err := client.ReadDeviceIdentification(ReadDeviceIDCodeRegular, 0x00)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Objects) != 3 {
		t.Fatalf("objects: expected merged 3, actual %d", len(result.Objects))
	}
	if len(h.requests) != 2 {
		t.Fatalf("requests: expected 2, actual %d", len(h.requests))
	}
	// Second request must continue from the returned Next Object Id (0x02).
	expectedReq2 := []byte{FuncCodeReadDeviceIdentification, MEITypeReadDeviceIdentification, ReadDeviceIDCodeRegular, 0x02}
	if !bytes.Equal(h.requests[1], expectedReq2) {
		t.Fatalf("second request: expected %x, actual %x", expectedReq2, h.requests[1])
	}
}

func TestReadDeviceIdentificationIndividual(t *testing.T) {
	h := &mockHandler{responses: [][]byte{
		// More Follows is set, but individual access must not continue.
		deviceIDResponse(ReadDeviceIDCodeIndividual, 0x83, 0xFF, 0x03, []idObject{
			{DeviceIDObjectMajorMinorRevision, []byte("1.2")},
		}),
	}}
	client := NewDeviceIdentificationClient(h)

	result, err := client.ReadDeviceIdentification(ReadDeviceIDCodeIndividual, DeviceIDObjectMajorMinorRevision)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.requests) != 1 {
		t.Fatalf("requests: expected 1 (no continuation), actual %d", len(h.requests))
	}
	if len(result.Objects) != 1 {
		t.Fatalf("objects: expected 1, actual %d", len(result.Objects))
	}
}

func TestReadDeviceIdentificationInvalidCode(t *testing.T) {
	for _, code := range []byte{0, 5, 0xFF} {
		h := &mockHandler{}
		client := NewDeviceIdentificationClient(h)
		_, err := client.ReadDeviceIdentification(code, 0x00)
		if err == nil {
			t.Fatalf("code %d: expected error, got nil", code)
		}
		if len(h.requests) != 0 {
			t.Fatalf("code %d: expected no request sent, sent %d", code, len(h.requests))
		}
	}
}

func TestReadDeviceIdentificationException(t *testing.T) {
	// Exception reply: function code 0x2B|0x80 = 0xAB, illegal function.
	h := &mockHandler{responses: [][]byte{
		{FuncCodeReadDeviceIdentification | 0x80, ExceptionCodeIllegalFunction},
	}}
	client := NewDeviceIdentificationClient(h)

	_, err := client.ReadDeviceIdentification(ReadDeviceIDCodeBasic, 0x00)
	if err == nil {
		t.Fatal("expected exception error, got nil")
	}
	var mbErr *ModbusError
	if !errors.As(err, &mbErr) {
		t.Fatalf("expected *ModbusError, got %T: %v", err, err)
	}
	if mbErr.ExceptionCode != ExceptionCodeIllegalFunction {
		t.Fatalf("exception code: expected %d, actual %d", ExceptionCodeIllegalFunction, mbErr.ExceptionCode)
	}
}

func TestReadDeviceIdentificationMalformed(t *testing.T) {
	// Too short: fewer than the 6 header bytes.
	t.Run("too short", func(t *testing.T) {
		h := &mockHandler{responses: [][]byte{
			{FuncCodeReadDeviceIdentification, MEITypeReadDeviceIdentification, ReadDeviceIDCodeBasic},
		}}
		_, err := NewDeviceIdentificationClient(h).ReadDeviceIdentification(ReadDeviceIDCodeBasic, 0x00)
		if err == nil {
			t.Fatal("expected error for short response, got nil")
		}
	})

	// Declared object count exceeds the objects actually present.
	t.Run("count mismatch", func(t *testing.T) {
		adu := deviceIDResponse(ReadDeviceIDCodeBasic, 0x81, 0x00, 0x00, []idObject{
			{DeviceIDObjectVendorName, []byte("Acme")},
		})
		adu[6] = 2 // claim 2 objects but only 1 is encoded
		h := &mockHandler{responses: [][]byte{adu}}
		_, err := NewDeviceIdentificationClient(h).ReadDeviceIdentification(ReadDeviceIDCodeBasic, 0x00)
		if err == nil {
			t.Fatal("expected error for count mismatch, got nil")
		}
	})
}

func TestReadDeviceIdentificationNonAdvancingStream(t *testing.T) {
	// Device signals More Follows but never advances the next object id.
	h := &mockHandler{responses: [][]byte{
		deviceIDResponse(ReadDeviceIDCodeRegular, 0x82, 0xFF, 0x00, []idObject{
			{DeviceIDObjectVendorName, []byte("Acme")},
		}),
	}}
	_, err := NewDeviceIdentificationClient(h).ReadDeviceIdentification(ReadDeviceIDCodeRegular, 0x00)
	if err == nil {
		t.Fatal("expected error for non-advancing stream, got nil")
	}
}

func TestReadDeviceIdentificationLoopCap(t *testing.T) {
	// Device always reports More Follows and always advances, so only the
	// hard message cap can stop the loop.
	h := &mockHandler{gen: func(reqIndex int, req []byte) []byte {
		return deviceIDResponse(ReadDeviceIDCodeRegular, 0x82, 0xFF, byte(reqIndex+1), []idObject{
			{byte(reqIndex), []byte{0x00}},
		})
	}}
	_, err := NewDeviceIdentificationClient(h).ReadDeviceIdentification(ReadDeviceIDCodeRegular, 0x00)
	if err == nil {
		t.Fatal("expected error when stream never completes, got nil")
	}
	if len(h.requests) != maxDeviceIDMessages {
		t.Fatalf("requests: expected cap %d, actual %d", maxDeviceIDMessages, len(h.requests))
	}
}

func TestDeviceIdentificationString(t *testing.T) {
	d := DeviceIdentification{
		ConformityLevel: 0x81,
		Objects: map[byte][]byte{
			DeviceIDObjectVendorName: []byte("Acme"),
			0x80:                     {0x00, 0xFF}, // non-printable -> hex fallback
		},
	}
	got := d.String()
	expected := "conformity level: 0x81\nobject 0x00: Acme\nobject 0x80: 00 ff"
	if got != expected {
		t.Fatalf("String():\nexpected %q\nactual   %q", expected, got)
	}
}

func TestDeviceIdentificationSortedIDs(t *testing.T) {
	d := DeviceIdentification{Objects: map[byte][]byte{
		0x80: {0x01},
		0x00: {0x02},
		0x06: {0x03},
		0x02: {0x04},
	}}
	got := d.SortedIDs()
	want := []byte{0x00, 0x02, 0x06, 0x80}
	if !bytes.Equal(got, want) {
		t.Fatalf("SortedIDs(): expected %x, actual %x", want, got)
	}
	// Empty objects must yield an empty (non-panicking) slice.
	if ids := (DeviceIdentification{}).SortedIDs(); len(ids) != 0 {
		t.Fatalf("SortedIDs() on empty: expected 0, actual %d", len(ids))
	}
}

func TestDeviceIDObjectName(t *testing.T) {
	named := map[byte]string{
		DeviceIDObjectVendorName:          "VendorName",
		DeviceIDObjectProductCode:         "ProductCode",
		DeviceIDObjectMajorMinorRevision:  "MajorMinorRevision",
		DeviceIDObjectVendorURL:           "VendorUrl",
		DeviceIDObjectProductName:         "ProductName",
		DeviceIDObjectModelName:           "ModelName",
		DeviceIDObjectUserApplicationName: "UserApplicationName",
	}
	for id, want := range named {
		if got := DeviceIDObjectName(id); got != want {
			t.Fatalf("DeviceIDObjectName(0x%02X): expected %q, actual %q", id, want, got)
		}
	}
	// Reserved and vendor-private ids have no standardized name.
	for _, id := range []byte{0x07, 0x40, 0x7F, 0x80, 0xFF} {
		if got := DeviceIDObjectName(id); got != "" {
			t.Fatalf("DeviceIDObjectName(0x%02X): expected \"\", actual %q", id, got)
		}
	}
}
