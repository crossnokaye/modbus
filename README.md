go modbus [![Build Status](https://travis-ci.org/goburrow/modbus.svg?branch=master)](https://travis-ci.org/goburrow/modbus) [![GoDoc](https://godoc.org/github.com/goburrow/modbus?status.svg)](https://godoc.org/github.com/goburrow/modbus)
=========
Fault-tolerant, fail-fast implementation of Modbus protocol in Go.

Supported functions
-------------------
Bit access:
*   Read Discrete Inputs
*   Read Coils
*   Write Single Coil
*   Write Multiple Coils

16-bit access:
*   Read Input Registers
*   Read Holding Registers
*   Write Single Register
*   Write Multiple Registers
*   Read/Write Multiple Registers
*   Mask Write Register
*   Read FIFO Queue

Device identification:
*   Read Device Identification (function 0x2B / MEI 0x0E, ID codes 1-4)

Supported formats
-----------------
*   TCP
*   Serial (RTU, ASCII)

Usage
-----
Basic usage:
```go
// Modbus TCP
client := modbus.TCPClient("localhost:502")
// Read input register 9
results, err := client.ReadInputRegisters(8, 1)

// Modbus RTU/ASCII
// Default configuration is 19200, 8, 1, even
client = modbus.RTUClient("/dev/ttyS0")
results, err = client.ReadCoils(2, 1)
```

Advanced usage:
```go
// Modbus TCP
handler := modbus.NewTCPClientHandler("localhost:502")
handler.Timeout = 10 * time.Second
handler.SlaveId = 0xFF
handler.Logger = log.New(os.Stdout, "test: ", log.LstdFlags)
// Connect manually so that multiple requests are handled in one connection session
err := handler.Connect()
defer handler.Close()

client := modbus.NewClient(handler)
results, err := client.ReadDiscreteInputs(15, 2)
results, err = client.WriteMultipleRegisters(1, 2, []byte{0, 3, 0, 4})
results, err = client.WriteMultipleCoils(5, 10, []byte{4, 3})
```

Read Device Identification over TCP:
```go
client := modbus.TCPClient("localhost:502", 10*time.Second, time.Minute)
// Read the basic identification objects (vendor name, product code, revision).
// Codes 1-3 stream and are gathered across "More Follows" continuations;
// code 4 reads a single object by id.
id, err := client.ReadDeviceIdentification(modbus.ReadDeviceIDCodeBasic, 0)
if err != nil {
	// Modbus exceptions (e.g. unsupported function) are returned as *modbus.ModbusError.
	log.Fatal(err)
}
fmt.Println(id) // prints conformity level and each returned object
```

```go
// Modbus RTU/ASCII
handler := modbus.NewRTUClientHandler("/dev/ttyUSB0")
handler.BaudRate = 115200
handler.DataBits = 8
handler.Parity = "N"
handler.StopBits = 1
handler.SlaveId = 1
handler.Timeout = 5 * time.Second

err := handler.Connect()
defer handler.Close()

client := modbus.NewClient(handler)
results, err := client.ReadDiscreteInputs(15, 2)
```

References
----------
-   [Modbus Specifications and Implementation Guides](http://www.modbus.org/specs.php)
