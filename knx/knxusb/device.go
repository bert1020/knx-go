package knxusb

import "github.com/vapourismo/knx-go/knx/cemi"

// A Device identifies a KNX USB HID interface. Path is passed to knx.NewUSB.
// IndividualAddress and MaxAPDULength are zero when the device does not
// provide them through local cEMI property reads.
type Device struct {
	Path              string
	Name              string
	Manufacturer      string
	ManufacturerCode  uint16
	IndividualAddress cemi.IndividualAddr
	MaxAPDULength     uint16
	ProbeError        string
}
