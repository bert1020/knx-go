[![Check](https://github.com/vapourismo/knx-go/actions/workflows/check.yaml/badge.svg?branch=master)](https://github.com/vapourismo/knx-go/actions/workflows/check.yaml)
[![GoDoc](https://godoc.org/github.com/vapourismo/knx-go?status.svg)](https://godoc.org/github.com/vapourismo/knx-go)

# knx-go

This repository contains a collection of Go packages that provide the means to communicate with KNX
networks.

## USB and device management

The existing `Tunnel` and `Router` implementations remain the IP transports. `USB` adds a cEMI
transport with the same `Send` and `Inbound` methods. A `Device` is opened from one of these
connections by physical address and exposes individual management operations. The implementation
uses Go and the KNX protocols directly; it does not load Falcon DLLs.

Connect through a KNXnet/IP tunneling gateway:

```go
transport, err := knx.NewGroupTunnel("192.168.1.10:3671", knx.DefaultTunnelConfig)
if err != nil { return err }
defer transport.Close()

address, err := cemi.NewIndividualAddrString("1.1.10")
if err != nil { return err }
device, err := transport.OpenDevice(ctx, address)
if err != nil { return err }
defer device.Close()

objects, err := device.ReadInterfaceObjects(ctx)
if err != nil { return err }
description, err := device.ReadPropertyDescriptionByID(ctx, 0, knx.PIDIOList)
if err != nil { return err }
value, err := device.ReadPropertyValue(ctx, 0, knx.PIDObjectType, 1, 1)
if err != nil { return err }
fmt.Printf("objects=%v property=%+v object-type=%x\n", objects, description, value)
```

For a writable property, pass its object index, property ID, element count, start index, and
encoded value bytes to `device.WritePropertyValue(ctx, objectIndex, propertyID, count, startIndex,
value)`. A successful call waits for and validates the device's property response.

For USB, call `ListUSBDevices()`, open a returned path with `NewUSB(ctx, path)`, then call
`usb.OpenDevice(ctx, address)`. An existing `Router` can call
`router.OpenDevice(ctx, source, address)`; routing requires a physical source address reserved for
the client. `OpenDeviceConnectionless` provides unnumbered management for devices that reject a
transport connection. `GroupTunnel.OpenDevice` shares the existing group tunnel with group
monitoring. A raw `Tunnel`, `Router`, or `USB` device session consumes that connection's cEMI
inbound stream, so use a dedicated connection when monitoring groups through those raw transports.

`DiscoverInterfaces(multicastAddress, timeout)` combines the existing IP `Discover` results with
USB HID enumeration. The original `Discover` and `DiscoverOnInterface` signatures remain available
for existing IP discovery callers.

`Device` exposes `ReadDeviceDescriptor`, `ReadPropertyElementCount`, `ReadPropertyValue`, `WritePropertyValue`,
`ReadPropertyDescriptionByID`, `ReadPropertyDescriptionByIndex`, `ReadInterfaceObjectType`, and
`ReadInterfaceObjects`. The object list comes from device object 0, PID 71 (`PID_IO_LIST`); a device
that does not implement that property will return an error. Property values remain raw bytes because
their property data type varies by device. These calls currently cover unencrypted management
telegram reads; authenticated properties, extended property services, and KNX Secure are not yet
implemented.

USB uses the standard KNX HID usage page and cEMI transfer protocol. It currently supports Windows
and Linux devices that advertise cEMI; Linux may require permission to open `/dev/hidraw*`.
`GroupUSB` is also available when only group communication is needed.

## Packages

 Package           | Description
-------------------|--------------------------------------------------------------------
 **knx**           | KNXnet/IP and USB connections, group communication, device management
 **knx/knxnet**    | KNXnet/IP protocol services
 **knx/dpt**       | Datapoint types
 **knx/cemi**      | CEMI-encoded frames
 **cmd/knxbridge** | Tool to bridge KNX networks between a KNXnet/IP router and gateway

## Installation

Simply run the following command.

	$ go get -u github.com/vapourismo/knx-go/...

## Examples

### KNXnet/IP Group Client

If you simply want to send and receive group communication, the
[GroupTunnel](https://godoc.org/github.com/vapourismo/knx-go/knx#GroupTunnel) or
[GroupRouter](https://godoc.org/github.com/vapourismo/knx-go/knx#GroupRouter)
might be sufficient to you.

```go
package main

import (
	"log"
	"os"

	"github.com/vapourismo/knx-go/knx"
	"github.com/vapourismo/knx-go/knx/cemi"
	"github.com/vapourismo/knx-go/knx/dpt"
	"github.com/vapourismo/knx-go/knx/util"
)

func main() {
	// Setup logger for auxiliary logging. This enables us to see log messages from internal
	// routines.
	util.Logger = log.New(os.Stdout, "", log.LstdFlags)

	// Connect to the gateway.
	client, err := knx.NewGroupTunnel("10.0.0.7:3671", knx.DefaultTunnelConfig)
	if err != nil {
		log.Fatal(err)
	}

	// Close upon exiting. Even if the gateway closes the connection, we still have to clean up.
	defer client.Close()

	// Send 20.5°C to group 1/2/3.
	err = client.Send(knx.GroupEvent{
		Command:     knx.GroupWrite,
		Destination: cemi.NewGroupAddr3(1, 2, 3),
		Data:        dpt.DPT_9001(20.5).Pack(),
	})
	if err != nil {
		log.Fatal(err)
	}

	// Receive messages from the gateway. The inbound channel is closed with the connection.
	for msg := range client.Inbound() {
		var temp dpt.DPT_9001

		err := temp.Unpack(msg.Data)
		if err != nil {
			continue
		}

		util.Logger.Printf("%+v: %v", msg, temp)
	}
}
```

In case you want to access a KNXnet/IP router instead of a gateway, simply replace

```go
client, err := knx.NewGroupTunnel("10.0.0.7:3671", knx.DefaultTunnelConfig)
```

with

```go
client, err := knx.NewGroupRouter("224.0.23.12:3671", knx.DefaultRouterConfig)
```

### KNXnet/IP CEMI Client

Use [Tunnel](https://godoc.org/github.com/vapourismo/knx-go/knx#Tunnel) or
[Router](https://godoc.org/github.com/vapourismo/knx-go/knx#Router) for finer control over the
communication with a gateway or router.

### KNX Bridge

The **knxbridge** tool (in package `cmd/knxbridge`) has multiple use cases.

Expose a KNX network behind a gateway at `10.0.0.2:3671` on the multicast group `224.0.23.12:3671`.
This allows routers and router clients to access the network.

	$ knxbridge 10.0.0.2:3671 224.0.23.12:3671

Connect two KNX networks through gateways. In this example one gateway is at `10.0.0.2:3671`, the
other is at `10.0.0.3:3671`.

	$ knxbridge 10.0.0.2:3671 10.0.0.3:3671

### Discover all KNXnet/IP Servers

The following example shows how to discover all routers/gateways on a network.

```go
package main

import (
	"log"
	"os"
	"time"

	"github.com/kr/pretty"

	"github.com/vapourismo/knx-go/knx"
	"github.com/vapourismo/knx-go/knx/util"
)

func main() {
	// Setup logger for auxiliary logging. This enables us to see log messages from internal
	// routines.
	util.Logger = log.New(os.Stdout, "", log.LstdFlags)

	servers, err := knx.Discover("224.0.23.12:3671", time.Millisecond*750)
	if err != nil {
		log.Fatal(err)
	}

	util.Logger.Printf("%# v", pretty.Formatter(servers))
}
```

### Describe a Single KNXnet/IP Server

The following example shows how to get a description from a single server.

```go
package main

import (
	"log"
	"os"
	"time"

	"github.com/kr/pretty"

	"github.com/vapourismo/knx-go/knx"
	"github.com/vapourismo/knx-go/knx/util"
)

func main() {
	util.Logger = log.New(os.Stdout, "", log.LstdFlags)

	// Describe KNXnet/IP server at given address and default port
	servers, err := knx.DescribeTunnel("192.168.1.254:3671", time.Millisecond*750)
	if err != nil {
		log.Fatal(err)
	}

	util.Logger.Printf("%# v", pretty.Formatter(servers))
}
```
