package knxusb

import (
	"errors"
	"os"
	"time"
)

// ErrUnsupported indicates that this operating system has no USB HID backend.
var ErrUnsupported = errors.New("KNX USB HID is currently supported on Windows and Linux")

// ErrWriteTimeout reports that an HID write attempt did not complete in
// time. The report was not delivered to the interface, so retrying it is safe.
var ErrWriteTimeout = errors.New("KNX USB HID write timed out")

// A Port abstracts the HID transport. Implementations must support I/O that
// can be cancelled through Cancel, so that a busy interface cannot block the
// caller indefinitely.
type Port interface {
	Read(b []byte) (int, error)
	Write(b []byte) (int, error)
	Cancel() error
	Close() error
}

// Per HID write attempt. Interfaces stall interrupt OUT transfers while
// they stream bus telegrams; cancelling and retrying once the interface
// is idle again beats waiting out the stall.
const writeTimeout = 750 * time.Millisecond

// A PortFile adapts a plain file (Linux hidraw) to Port. Linux hidraw
// writes do not exhibit the long stalls seen on Windows, so blocking file I/O
// is sufficient there and Cancel is a no-op.
type PortFile struct{ *os.File }

func (PortFile) Cancel() error { return nil }
