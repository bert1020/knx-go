package knx

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/vapourismo/knx-go/knx/cemi"
	"github.com/vapourismo/knx-go/knx/knxnet"
)

// Run with: go test ./knx -run '^TestTunnelReadDeviceInfoLive$' -v -count=1
//
// Connects to a KNXnet/IP tunneling gateway and reads the device descriptor
// and device object properties of the device at 1.1.2 through the tunnel,
// using a connected TP1 management session.
func TestTunnelReadDeviceInfoLive(t *testing.T) {
	const target = cemi.IndividualAddr(0x1102) // 1.1.2
	tunnel, err := NewTunnel("192.168.123.200:3671", knxnet.TunnelLayerData, TunnelConfig{})
	if err != nil {
		t.Fatalf("connect to 192.168.123.200:3671: %v", err)
	}
	defer tunnel.Close()
	t.Logf("tunnel established")

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	dev, err := tunnel.OpenDevice(ctx, target)
	if err != nil {
		t.Fatalf("connect to 1.1.2: %v", err)
	}
	defer dev.Close()
	t.Logf("1.1.2 connected over tunnel")

	if desc, err := dev.ReadDeviceDescriptor(ctx); err != nil {
		t.Errorf("1.1.2 device descriptor: %v", err)
	} else {
		t.Logf("1.1.2 device descriptor: 0x%04X (mask version %d.%d)", desc, desc>>8, desc&0xff)
	}

	for _, p := range []struct {
		name string
		id   uint8
	}{
		{"serial number", 11},
		{"manufacturer code", 12},
		{"device control", 51},
		{"max APDU length", 56},
		{"subnet address", 57},
		{"device address", 58},
	} {
		reqCtx, cancelReq := context.WithTimeout(ctx, 5*time.Second)
		value, err := dev.ReadPropertyValue(reqCtx, 0, p.id, 1, 1)
		cancelReq()
		if err != nil {
			t.Errorf("1.1.2 %s (PID %d): %v", p.name, p.id, err)
			continue
		}
		switch p.id {
		case 12, 56:
			t.Logf("1.1.2 %s: 0x%04X", p.name, binary.BigEndian.Uint16(value))
		default:
			t.Logf("1.1.2 %s: % X", p.name, value)
		}
	}
}

// Run with: go test ./knx -run '^TestTunnelWriteDeviceManufacturerLive$' -v -count=1
//
// Writes the current manufacturer code of the device at 1.1.2 back to it
// through the tunneling gateway and reads it back, verifying the write path
// end to end without changing the device. Skips when the property is write
// protected.
func TestTunnelWriteDeviceManufacturerLive(t *testing.T) {
	const target = cemi.IndividualAddr(0x1102) // 1.1.2
	tunnel, err := NewTunnel("192.168.123.200:3671", knxnet.TunnelLayerData, TunnelConfig{})
	if err != nil {
		t.Fatalf("connect to 192.168.123.200:3671: %v", err)
	}
	defer tunnel.Close()
	t.Logf("tunnel established")

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	dev, err := tunnel.OpenDevice(ctx, target)
	if err != nil {
		t.Fatalf("connect to 1.1.2: %v", err)
	}
	defer dev.Close()
	t.Logf("1.1.2 connected over tunnel")

	before, err := dev.ReadPropertyValue(ctx, 0, PIDDeviceManufacturer, 1, 1)
	if err != nil {
		t.Fatalf("read current manufacturer code: %v", err)
	}
	if len(before) != 2 {
		t.Fatalf("manufacturer code has %d bytes: % X", len(before), before)
	}
	code := binary.BigEndian.Uint16(before)
	t.Logf("1.1.2 current manufacturer code: 0x%04X", code)

	desc, err := dev.ReadPropertyDescriptionByID(ctx, 0, PIDDeviceManufacturer)
	if err != nil {
		t.Fatalf("read manufacturer code description: %v", err)
	}
	t.Logf("1.1.2 manufacturer code description: write enabled %v, data type %d, max elements %d, write level %d",
		desc.WriteEnabled, desc.PropertyDataType, desc.MaxElements, desc.WriteLevel)
	if !desc.WriteEnabled {
		t.Skip("1.1.2 manufacturer code is write protected")
	}

	if err := dev.WriteDeviceManufacturer(ctx, code); err != nil {
		t.Fatalf("write manufacturer code: %v", err)
	}
	t.Logf("1.1.2 wrote manufacturer code 0x%04X", code)

	after, err := dev.ReadPropertyValue(ctx, 0, PIDDeviceManufacturer, 1, 1)
	if err != nil {
		t.Fatalf("read back manufacturer code: %v", err)
	}
	if len(after) != 2 {
		t.Fatalf("read back manufacturer code has %d bytes: % X", len(after), after)
	}
	if got := binary.BigEndian.Uint16(after); got != code {
		t.Fatalf("manufacturer code changed: got 0x%04X, want 0x%04X", got, code)
	}
	t.Logf("1.1.2 read back manufacturer code: 0x%04X", code)
}
