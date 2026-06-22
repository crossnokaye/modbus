## Context

The package exposes a transport-agnostic `Client` interface (`api.go`) implemented by `client`
(`client.go`). Every operation builds a `ProtocolDataUnit{FunctionCode, Data}` and calls
`mb.send`, which encodes via the `Packager`, transmits via the `Transporter`, verifies and decodes
the response, and — crucially — maps any response whose function code differs from the request into a
`*ModbusError` via `responseError`. Read Device Identification (function `0x2B`, MEI type `0x0E`) is
an Encapsulated Interface Transport function with a different request/response shape than the
register/coil functions, but it rides the exact same `send` pipeline, so exception detection is
already handled. The change is additive: a new method plus decode helpers and constants, with the
primary target being Modbus/TCP.

## Goals / Non-Goals

**Goals:**
- Add `Client.ReadDeviceIdentification` issuing function `0x2B` / MEI `0x0E`.
- Support Read Device ID codes 1-4, with auto-continuation across "More Follows" for stream codes.
- Return a structured, printable result (conformity level + objects keyed by object id).
- Surface Modbus exceptions to this request as `*ModbusError`, reusing the existing pipeline.

**Non-Goals:**
- No CLI/binary or example program — this delivers a library method plus a formatting helper; the
  caller does the printing.
- No server-side (responder) implementation of function `0x2B`.
- No interpretation of vendor-specific object semantics beyond exposing raw bytes (object ids 0-2
  are well-known strings; others are returned as-is).

## Decisions

### Decision: Expose via an extension interface, NOT by changing `Client`
Implement `ReadDeviceIdentification` on the concrete `*client` and expose it through a new
`DeviceIdentificationClient` interface that embeds `Client` and adds the method, plus a
`NewDeviceIdentificationClient(handler)` constructor. The existing `Client` interface is left
untouched.
- **Why**: `Client` is implemented outside this repo — notably `hardware-drivers` has GoMock-generated
  `MockClient`s (`protocols/abb`, `protocols/acquilite`), a hand-written `sharedClient`, and a test
  `mockClient`, all of which satisfy `Client`. Adding a method to `Client` would stop them satisfying
  it and break their build. Keeping `Client` byte-for-byte identical avoids that entirely.
- **How callers reach it**: `NewDeviceIdentificationClient(handler)` returns the extension interface
  directly; alternatively a `Client` from `NewClient`/`TCPClient` can be type-asserted to
  `DeviceIdentificationClient` (the concrete `*client` implements it). This is the standard Go
  interface-evolution pattern (cf. `http.Pusher`, `io.ReaderFrom`).
- **Alternative considered**: add the method to `Client`. Rejected — breaks external implementers
  (hard constraint). A free function was also rejected because the request needs the unexported
  `send` pipeline, not reachable through the `Client` interface alone.

### Decision: Structured, printable result type (map keyed by object id)
Introduce an exported result type:

```go
type DeviceIdentification struct {
    ConformityLevel byte
    Objects         map[byte][]byte // object id -> raw value bytes
}
func (d DeviceIdentification) SortedIDs() []byte // object ids in ascending order
func (d DeviceIdentification) String() string    // sorted, human-readable listing
```

Returning a struct (rather than raw `[]byte`) is necessary because the response is a list of
variable-length objects. A **map keyed by object id** (not an ordered slice) is chosen deliberately
because the primary consumer sweeps all stream codes (1, 2, 3) and merges their overlapping results
by object id — a map merges idempotently in one line, whereas a slice would need dedup-by-id on every
merge. `SortedIDs()` restores deterministic ordering for display, and `String()` satisfies the
"prints the returned identification objects" requirement. Naming is provided separately by
`DeviceIDObjectName` (below) rather than embedded in the result, so the map stays merge-friendly and
the name table is not duplicated.
- **Alternative considered**: an ordered `[]DeviceIDObject{ID, Name, Value}`. Rejected — cleaner for a
  single-code read, but requires dedup-by-id across the multi-code sweep and embeds a Name that
  duplicates `DeviceIDObjectName`.
- **Alternative considered**: returning `[]byte` like other methods. Rejected — loses object
  framing and would push re-parsing onto every caller.

### Decision: Spec-standard object names live in the library
Expose `DeviceIDObjectName(id byte) string` returning the Modbus-standard name for well-known object
ids `0x00`–`0x06` (VendorName … UserApplicationName), and `""` for reserved (`0x07`–`0x7F`) or
vendor-private (`0x80`–`0xFF`) ids. The id→name mapping is defined by the Modbus spec — universal and
fixed — so it belongs in the shared library as a single source of truth rather than being
re-maintained in each client (netkit, hardware-drivers, …). Device/vendor-specific interpretation of
private objects stays in the client that owns that knowledge.

### Decision: Auto-loop on "More Follows" for stream codes
For codes 1-3, after decoding each response, if `MoreFollows == 0xFF` the client re-issues the
request with `objectID = NextObjectId` and merges objects into the same map until `MoreFollows`
clears (`0x00`). Code 4 (individual access) sends exactly one request and never loops. The loop is
bounded by a maximum iteration count (objects ids are a single byte, so ≤256 messages is a safe hard
cap) to defend against a device that never clears More Follows or fails to advance Next Object Id.
- **Alternative considered**: return one message plus a "more" flag and let the caller continue.
  Rejected per the chosen behavior — callers want the complete set; looping is spec-correct and the
  partial-result mode adds API surface for little benefit.

### Decision: Reuse `send` for exception handling
Build the request PDU and call `mb.send`. Because the exception response carries function code
`0xAB` (`0x2B | 0x80`), `send` already returns a `*ModbusError`; no new exception logic is needed.
The method only adds shape validation of the *successful* response (MEI type echo, declared object
count vs. bytes present, per-object length bounds) and returns descriptive errors on malformation.
- **Alternative considered**: bespoke exception parsing in the new method. Rejected — duplicates
  existing, tested behavior.

### Decision: New constants in `modbus.go`
Add `FuncCodeReadDeviceIdentification = 0x2B`, `MEITypeReadDeviceIdentification = 0x0E`, the four
`ReadDeviceIDCode*` values, and the well-known object-id constants, alongside existing function-code
and exception-code constants for discoverability.

## Risks / Trade-offs

- **Adding a method to `Client` breaks external implementers** → Document as BREAKING in the
  proposal/release notes; in-tree the only implementer is `client`, so the build stays green.
- **Device never clears More Follows / fails to advance Next Object Id (infinite loop)** → Bound the
  loop with a hard iteration cap (≤256) and return an error if exceeded or if Next Object Id does not
  progress.
- **Devices with partial conformity may not return all requested objects, or reject higher codes
  with an exception** → Return whatever objects arrive; an exception is surfaced as `*ModbusError`
  for the caller to interpret (e.g. device only supports basic identification).
- **Object values are vendor-defined bytes** → Expose raw bytes; `String()` renders printable ASCII
  and falls back to hex so non-text objects remain readable.

## Migration Plan

Purely additive at the call-site level. Existing callers are unaffected. Any external code that
implements `Client` directly must add the new method to compile; the recommended path is to use the
provided handlers (`NewClient`/`TCPClient`) which gain the method automatically.

## Open Questions

- None blocking. The hard loop cap (256) and the `String()` rendering format (ASCII-with-hex
  fallback) are implementation defaults that can be tuned during review without changing the spec.
