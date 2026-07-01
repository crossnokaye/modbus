## Why

The library implements the common Modbus data-access functions but has no way to interrogate a
device's identity (vendor, product code, firmware revision, etc.). Operators integrating new
hardware over Modbus/TCP need a programmatic way to read these identification objects so they can
inventory devices, verify firmware, and confirm a device is the model they expect before issuing
control operations.

## What Changes

- Add a new `ReadDeviceIdentification` method to the `Client` API that issues a Modbus Read Device
  Identification request (function code `0x2B`, MEI type `0x0E`).
- Support all four Read Device ID access codes: `1` (basic), `2` (regular), `3` (extended) — which
  use stream access — and `4` (individual access to a single specified object).
- Transparently handle the "More Follows" continuation: for stream-access codes the client loops,
  re-issuing the request from the returned Next Object Id until the device reports no more objects,
  and returns the complete, merged set of objects.
- Return a structured result (conformity level + the identification objects keyed by object id)
  together with a human-readable formatting helper so callers can print the returned objects.
- Detect and surface Modbus exception responses to this request (e.g. illegal function, illegal
  data value) as the existing `*ModbusError`, with no special-casing required by callers.
- Add the new function code / MEI type / Read Device ID code / conformity constants to the package.
- Expose the method WITHOUT modifying the existing `Client` interface: add a `DeviceIdentificationClient`
  extension interface (embeds `Client` + the new method) plus a `NewDeviceIdentificationClient`
  constructor. This keeps `Client` byte-for-byte unchanged so existing implementers/mocks (e.g. in
  `hardware-drivers`) keep compiling — **non-breaking**.

## Capabilities

### New Capabilities
- `read-device-identification`: Issuing a Modbus Read Device Identification request over a connected
  client, decoding the returned identification objects across access codes 1-4, auto-continuing the
  "More Follows" stream, and reporting Modbus exception responses.

### Modified Capabilities
<!-- None: no existing capability specs in openspec/specs/. -->

## Impact

- **Code**: All new code isolated in `deviceidentification.go` (constants, `DeviceIdentification`
  type + `String()`, `DeviceIdentificationClient` interface, `NewDeviceIdentificationClient`
  constructor, the method on `*client`, and the decode/loop helpers) and `deviceidentification_test.go`.
  `api.go`/`client.go`/`modbus.go` are unchanged. `README.md` supported-functions list updated.
- **APIs**: New public `DeviceIdentificationClient` interface, `NewDeviceIdentificationClient`
  constructor, `ReadDeviceIdentification` method (on the concrete client), and a new exported result
  type + constants. The existing `Client` interface and all existing method signatures are unchanged.
- **Dependencies**: None — reuses the existing `Packager`/`Transporter`/`send` pipeline, which
  already maps mismatched response function codes to `*ModbusError`.
- **Transport**: Works over any transport (TCP/RTU/ASCII) since the request is a standard PDU; the
  primary target and test focus is Modbus/TCP.
