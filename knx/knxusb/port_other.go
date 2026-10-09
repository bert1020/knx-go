//go:build !linux && !windows

package knxusb

// OpenPort returns ErrUnsupported on operating systems without a USB HID backend.
func OpenPort(path string) (Port, error) {
	return nil, ErrUnsupported
}

// ListDevices returns ErrUnsupported on operating systems without a USB HID backend.
func ListDevices() ([]Device, error) {
	return nil, ErrUnsupported
}
