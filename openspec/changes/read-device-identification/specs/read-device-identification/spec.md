## ADDED Requirements

### Requirement: Issue Read Device Identification request

The client SHALL provide a method that issues a Modbus Read Device Identification request using
function code `0x2B` and MEI type `0x0E`. The request SHALL carry the caller-supplied Read Device ID
code and Object Id. The method SHALL be available on the `Client` interface so it works over any
configured transport, with Modbus/TCP as the primary target.

#### Scenario: Basic identification request is encoded correctly

- **WHEN** the caller invokes the method with Read Device ID code `0x01` (basic) and object id `0x00`
- **THEN** the client SHALL send a PDU whose function code is `0x2B` and whose data is
  `[0x0E, 0x01, 0x00]` (MEI type, Read Device ID code, Object Id)

### Requirement: Validate the Read Device ID code

The method SHALL accept only Read Device ID codes `1` (basic), `2` (regular), `3` (extended), and
`4` (individual). Any other code SHALL be rejected with an error before any request is sent.

#### Scenario: Supported codes are accepted

- **WHEN** the caller supplies a Read Device ID code of 1, 2, 3, or 4
- **THEN** the client SHALL build and send the request

#### Scenario: Unsupported code is rejected

- **WHEN** the caller supplies a Read Device ID code outside the range 1-4
- **THEN** the client SHALL return an error and SHALL NOT send a request

### Requirement: Decode returned identification objects

The client SHALL decode the response into a structured result containing the device conformity level
and the returned identification objects, each keyed by its object id with its raw value bytes
preserved. The result SHALL expose the objects so a caller can print them.

#### Scenario: Objects are parsed from a response

- **WHEN** the device returns a response listing objects (each as object id, length, and value bytes)
- **THEN** the result SHALL contain every returned object keyed by object id with its value bytes
  intact, along with the reported conformity level

#### Scenario: Result can be rendered for display

- **WHEN** a caller renders the decoded result with the provided formatting helper
- **THEN** the output SHALL list each returned object id alongside its value in a human-readable form

#### Scenario: Objects can be iterated in deterministic order

- **WHEN** a caller requests the object ids of the result for ordered iteration or display
- **THEN** the client SHALL return the present object ids in a stable ascending order independent of
  map iteration order

### Requirement: Provide standard object names

The library SHALL provide a lookup from object id to the Modbus-standard object name for the
well-known ids `0x00`-`0x06`, returning an empty name for reserved (`0x07`-`0x7F`) or vendor-private
(`0x80`-`0xFF`) ids, so consumers do not maintain their own copy of this spec-defined table.

#### Scenario: Well-known id resolves to its standard name

- **WHEN** a caller looks up the name for a well-known object id such as `0x00`
- **THEN** the library SHALL return its standard name (e.g. `VendorName`)

#### Scenario: Unknown id resolves to an empty name

- **WHEN** a caller looks up the name for a reserved or vendor-private object id (e.g. `0x80`)
- **THEN** the library SHALL return an empty name

### Requirement: Continue stream access until complete

For stream-access codes (1, 2, 3), the client SHALL automatically continue reading when a response
indicates "More Follows" (`0xFF`): it MUST issue follow-up requests starting at the returned Next
Object Id and merge all returned objects, until the device reports no more objects remain. The
continuation SHALL be bounded so a misbehaving device cannot cause an unbounded loop.

#### Scenario: Multi-message stream is reassembled

- **WHEN** a stream-access response sets More Follows to `0xFF` with a Next Object Id
- **THEN** the client SHALL issue a follow-up request from that Next Object Id and SHALL return the
  combined set of objects from all messages once More Follows is cleared (`0x00`)

#### Scenario: Individual access does not continue

- **WHEN** the caller uses Read Device ID code `4` (individual access)
- **THEN** the client SHALL issue a single request for the specified object and SHALL NOT loop on
  More Follows

### Requirement: Surface Modbus exception responses

The client SHALL detect a Modbus exception reply to the Read Device Identification request and return
it to the caller as a `*ModbusError` carrying the function code and exception code, rather than
returning a partial or malformed result.

#### Scenario: Exception response is reported as an error

- **WHEN** the device replies with the exception function code (`0x2B | 0x80`) and an exception code
  such as illegal function or illegal data value
- **THEN** the client SHALL return a `*ModbusError` with that exception code and SHALL NOT return
  decoded identification objects

#### Scenario: Malformed response is rejected

- **WHEN** the device returns a response that is too short or whose declared object count does not
  match the bytes received
- **THEN** the client SHALL return a descriptive error rather than partial objects
