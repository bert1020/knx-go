package knx

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/vapourismo/knx-go/knx/cemi"
)

// Run with: go test ./knx -run '^TestUSBReadDeviceInfoLive$' -v -count=1
//
// Reads the device descriptor and device object properties of the device at
// 1.1.2 through the first discovered KNX USB interface, using a connected
// TP1 management session. Many devices only answer management requests over
// a transport connection and only when the frames carry no link-layer ack
// request bit, which the library already ensures. The USB interface must not
// be held by another application (e.g. ETS).
func TestUSBReadDeviceInfoLive(t *testing.T) {
	const target = cemi.IndividualAddr(0x1102) // 1.1.2
	devices, err := ListUSBDevices()
	if errors.Is(err, ErrUSBUnsupported) {
		t.Skip("USB discovery is unsupported on this operating system")
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) == 0 {
		t.Skip("no KNX USB interface found")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	u, err := NewUSB(ctx, devices[0].Path)
	if err != nil {
		t.Fatalf("open %s: %v", devices[0].Path, err)
	}
	defer u.Close()

	dev, err := u.OpenDevice(ctx, target)
	if err != nil {
		t.Fatalf("connect to 1.1.2: %v", err)
	}
	defer dev.Close()
	t.Logf("1.1.2 connected over USB")

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

// Run with: go test ./knx -run '^TestUSBWriteDeviceManufacturerLive$' -v -count=1
//
// Writes the current manufacturer code of the device at 1.1.2 back to it and
// reads it back, verifying the write path end to end without changing the
// device. Skips when the property is write protected. The USB interface must
// not be held by another application (e.g. ETS).
func TestUSBWriteDeviceManufacturerLive(t *testing.T) {
	const target = cemi.IndividualAddr(0x1102) // 1.1.2
	devices, err := ListUSBDevices()
	if errors.Is(err, ErrUSBUnsupported) {
		t.Skip("USB discovery is unsupported on this operating system")
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) == 0 {
		t.Skip("no KNX USB interface found")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	u, err := NewUSB(ctx, devices[0].Path)
	if err != nil {
		t.Fatalf("open %s: %v", devices[0].Path, err)
	}
	defer u.Close()

	dev, err := u.OpenDevice(ctx, target)
	if err != nil {
		t.Fatalf("connect to 1.1.2: %v", err)
	}
	defer dev.Close()
	t.Logf("1.1.2 connected over USB")

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
