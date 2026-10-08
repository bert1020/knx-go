//go:build !linux && !windows

package knx

func listUSBDevices() ([]USBDevice, error) {
	return nil, ErrUSBUnsupported
}
