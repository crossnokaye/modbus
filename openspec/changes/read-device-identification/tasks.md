## 1. Constants and types

- [x] 1.1 Add `FuncCodeReadDeviceIdentification = 0x2B` and `MEITypeReadDeviceIdentification = 0x0E` constants (in isolated `deviceidentification.go`)
- [x] 1.2 Add Read Device ID code constants (`ReadDeviceIDCodeBasic = 1`, `Regular = 2`, `Extended = 3`, `Individual = 4`)
- [x] 1.3 Add well-known object-id constants (VendorName=0x00, ProductCode=0x01, MajorMinorRevision=0x02, plus VendorURL/ProductName/ModelName/UserApplicationName)
- [x] 1.4 Define exported `DeviceIdentification` struct (`ConformityLevel byte`, `Objects map[byte][]byte`) with `SortedIDs()` (deterministic order) and a `String()` method that renders each object id and value (printable ASCII, hex fallback)
- [x] 1.5 Add `DeviceIDObjectName(id byte) string` returning the spec-standard name for well-known ids `0x00`-`0x06` (`""` otherwise), so consumers don't re-maintain the id→name table

## 2. Client method

- [x] 2.1 Expose `ReadDeviceIdentification` WITHOUT changing `Client`: add a `DeviceIdentificationClient` extension interface (embeds `Client` + the method) and a `NewDeviceIdentificationClient` constructor; method lives on the concrete `*client` (non-breaking for external `Client` implementers like hardware-drivers)
- [x] 2.2 Implement the method on `client` (isolated in `deviceidentification.go`): validate `readDeviceIDCode` is 1-4 (error before sending otherwise)
- [x] 2.3 Build the request PDU (`FunctionCode 0x2B`, data `[0x0E, code, objectID]`) and call `mb.send` so exceptions map to `*ModbusError`
- [x] 2.4 Write a response-decode helper that validates the MEI type echo and conformity level, parses the object list (object id, length, value), and validates declared object count vs. bytes present
- [x] 2.5 For codes 1-3, loop on `MoreFollows == 0xFF`: re-issue from `NextObjectId`, merge objects into the result map; for code 4 send a single request
- [x] 2.6 Bound the continuation loop with a hard cap (≤256 messages) and error if exceeded or if `NextObjectId` fails to advance

## 3. Tests

- [x] 3.1 Add a mock/stub `Transporter`/`Packager` (`mockHandler` in `deviceidentification_test.go`) to drive `ReadDeviceIdentification` without a live device
- [x] 3.2 Test successful basic (code 1) single-message decode produces expected objects and conformity level
- [x] 3.3 Test multi-message stream: first response sets More Follows=0xFF, follow-up completes; assert merged object set and correct Next Object Id used
- [x] 3.4 Test individual access (code 4) sends one request and returns the single requested object
- [x] 3.5 Test invalid Read Device ID code is rejected before any send
- [x] 3.6 Test exception response (function `0xAB`, e.g. illegal function / illegal data value) returns a `*ModbusError` with the right exception code
- [x] 3.7 Test malformed responses (too short, count mismatch) return descriptive errors; test the loop cap (non-advancing + hard cap) guards against a device that never clears More Follows

## 4. Documentation

- [x] 4.1 Add "Read Device Identification" to the supported-functions list in `README.md` with a short TCP usage snippet
- [x] 4.2 Run `go build ./...`, `go vet ./...`, and `go test ./...` — root package builds/vets/passes (pre-existing `./test` integration tests require live hardware/server and are unaffected)
