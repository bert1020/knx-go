package knx

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"github.com/vapourismo/knx-go/knx/cemi"
)

const (
	usbPropReadRequest  cemi.MessageCode = 0xfc
	usbPropReadConfirm  cemi.MessageCode = 0xfb
	usbDeviceObjectType                  = 0
	usbDeviceInstance                    = 1
	usbPIDManufacturer                   = 12
	usbPIDMaxAPDULength                  = 56
	usbPIDSubnetAddress                  = 57
	usbPIDDeviceAddress                  = 58
)

// ReadLocalProperty reads a property of the USB interface itself through
// cEMI M_PropRead. Do not consume Inbound concurrently with this call.
func (u *USB) ReadLocalProperty(ctx context.Context, objectType uint16, instance, propertyID, count uint8, startIndex uint16) ([]byte, error) {
	if ctx == nil {
		return nil, errors.New("nil context")
	}
	if u == nil {
		return nil, errors.New("nil USB connection")
	}
	if count == 0 || count > 15 || startIndex > 0x0fff {
		return nil, errors.New("invalid property count or start index")
	}
	u.localMu.Lock()
	defer u.localMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	request := []byte{byte(objectType >> 8), byte(objectType), instance, propertyID, count<<4 | byte(startIndex>>8), byte(startIndex)}
	if err := u.Send(&cemi.UnsupportedMessage{Code: usbPropReadRequest, Data: request}); err != nil {
		return nil, err
	}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-u.done:
			return nil, errors.New("USB connection is closed")
		case message, ok := <-u.Inbound():
			if !ok {
				return nil, errors.New("USB inbound stream is closed")
			}
			response, ok := message.(*cemi.UnsupportedMessage)
			if !ok || response.Code != usbPropReadConfirm || len(response.Data) < 6 {
				continue
			}
			if binary.BigEndian.Uint16(response.Data[:2]) != objectType || response.Data[2] != instance || response.Data[3] != propertyID ||
				uint16(response.Data[4]&15)<<8|uint16(response.Data[5]) != startIndex {
				continue
			}
			if response.Data[4]>>4 == 0 {
				if len(response.Data) > 6 {
					return nil, fmt.Errorf("local property %d rejected with code 0x%02x", propertyID, response.Data[6])
				}
				return nil, fmt.Errorf("local property %d rejected", propertyID)
			}
			if response.Data[4]>>4 != count {
				return nil, errors.New("local property response element count mismatch")
			}
			return append([]byte(nil), response.Data[6:]...), nil
		}
	}
}

func probeUSBDevice(ctx context.Context, device *USBDevice) {
	u, err := NewUSB(ctx, device.Path)
	if err != nil {
		device.ProbeError = err.Error()
		return
	}
	defer u.Close()
	read := func(pid uint8, expected int) []byte {
		value, err := u.ReadLocalProperty(ctx, usbDeviceObjectType, usbDeviceInstance, pid, 1, 1)
		if err != nil || len(value) != expected {
			if err == nil {
				err = fmt.Errorf("expected %d bytes, got %d", expected, len(value))
			}
			if device.ProbeError != "" {
				device.ProbeError += "; "
			}
			device.ProbeError += fmt.Sprintf("property %d: %v", pid, err)
			return nil
		}
		return value
	}
	if value := read(usbPIDManufacturer, 2); value != nil {
		device.ManufacturerCode = binary.BigEndian.Uint16(value)
		if device.Manufacturer == "" {
			device.Manufacturer = fmt.Sprintf("KNX 0x%04X", device.ManufacturerCode)
		}
	}
	subnet := read(usbPIDSubnetAddress, 1)
	address := read(usbPIDDeviceAddress, 1)
	if subnet != nil && address != nil {
		device.IndividualAddress = cemi.IndividualAddr(uint16(subnet[0])<<8 | uint16(address[0]))
	}
	if value := read(usbPIDMaxAPDULength, 2); value != nil {
		device.MaxAPDULength = binary.BigEndian.Uint16(value)
	}
	device.ProbeError = strings.TrimSpace(device.ProbeError)
}
