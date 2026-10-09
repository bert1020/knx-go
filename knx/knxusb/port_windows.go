//go:build windows

package knxusb

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

var (
	hidDLL       = syscall.NewLazyDLL("hid.dll")
	setupDLL     = syscall.NewLazyDLL("setupapi.dll")
	hidGuid      = hidDLL.NewProc("HidD_GetHidGuid")
	hidPreparsed = hidDLL.NewProc("HidD_GetPreparsedData")
	hidFree      = hidDLL.NewProc("HidD_FreePreparsedData")
	hidCaps      = hidDLL.NewProc("HidP_GetCaps")
	hidProduct   = hidDLL.NewProc("HidD_GetProductString")
	hidMaker     = hidDLL.NewProc("HidD_GetManufacturerString")
	setupGet     = setupDLL.NewProc("SetupDiGetClassDevsW")
	setupEnum    = setupDLL.NewProc("SetupDiEnumDeviceInterfaces")
	setupDetail  = setupDLL.NewProc("SetupDiGetDeviceInterfaceDetailW")
	setupClose   = setupDLL.NewProc("SetupDiDestroyDeviceInfoList")

	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	procCreateEvent   = kernel32.NewProc("CreateEventW")
	procOverlapResult = kernel32.NewProc("GetOverlappedResult")
)

const (
	waitObject0  = 0x00000000
	waitTimeout  = 0x00000102
	waitInfinite = 0xFFFFFFFF
)

// OpenPort opens the HID device in overlapped mode. Synchronous HID writes
// block for seconds while the interface streams bus telegrams, which freezes
// property reads and data sends; overlapped I/O keeps those cancellable.
func OpenPort(path string) (Port, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(p,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE,
		nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return nil, fmt.Errorf("open KNX USB HID: %w", err)
	}
	return &portWindows{h: h}, nil
}

// portWindows runs overlapped HID I/O on a raw handle. Write waits at most
// writeTimeout and cancels the stalled transfer; the caller retries once
// the interface stopped streaming. Read blocks until data arrives or Cancel
// aborts the transfer during Close.
type portWindows struct {
	h syscall.Handle
}

func (p *portWindows) prepare() (*syscall.Overlapped, func(), error) {
	// manual-reset event, initially not signalled
	event, _, callErr := procCreateEvent.Call(0, 1, 0, 0)
	if event == 0 {
		return nil, nil, callErr
	}
	ol := &syscall.Overlapped{HEvent: syscall.Handle(event)}
	return ol, func() { syscall.CloseHandle(syscall.Handle(event)) }, nil
}

func (p *portWindows) wait(ol *syscall.Overlapped, timeout uint32) (uint32, error) {
	state, err := syscall.WaitForSingleObject(ol.HEvent, timeout)
	if state != waitObject0 {
		if state == 0xFFFFFFFF {
			return 0, err
		}
		if state == waitTimeout {
			syscall.CancelIoEx(p.h, ol)
			return 0, ErrWriteTimeout
		}
		return 0, fmt.Errorf("WaitForSingleObject: state %d, %v", state, err)
	}
	var done uint32
	ok, _, callErr := procOverlapResult.Call(uintptr(p.h), uintptr(unsafe.Pointer(ol)), uintptr(unsafe.Pointer(&done)), 0)
	if ok == 0 {
		return 0, callErr
	}
	return done, nil
}

func (p *portWindows) Read(b []byte) (int, error) {
	ol, cleanup, err := p.prepare()
	if err != nil {
		return 0, err
	}
	defer cleanup()
	var done uint32
	err = syscall.ReadFile(p.h, b, &done, ol)
	if err == syscall.ERROR_IO_PENDING {
		done, err = p.wait(ol, waitInfinite)
	}
	return int(done), err
}

func (p *portWindows) Write(b []byte) (int, error) {
	ol, cleanup, err := p.prepare()
	if err != nil {
		return 0, err
	}
	defer cleanup()
	var done uint32
	err = syscall.WriteFile(p.h, b, &done, ol)
	if err == syscall.ERROR_IO_PENDING {
		done, err = p.wait(ol, uint32(writeTimeout/time.Millisecond))
	}
	return int(done), err
}

func (p *portWindows) Cancel() error {
	return syscall.CancelIoEx(p.h, nil)
}

func (p *portWindows) Close() error {
	return syscall.CloseHandle(p.h)
}

type deviceInterfaceData struct {
	Size      uint32
	ClassGUID syscall.GUID
	Flags     uint32
	Reserved  uintptr
}

// ListDevices enumerates present HID devices that speak the KNX USB protocol.
func ListDevices() ([]Device, error) {
	var guid syscall.GUID
	hidGuid.Call(uintptr(unsafe.Pointer(&guid)))
	set, _, e := setupGet.Call(uintptr(unsafe.Pointer(&guid)), 0, 0, 0x12) // PRESENT | DEVICEINTERFACE
	if set == ^uintptr(0) {
		return nil, fmt.Errorf("SetupDiGetClassDevsW: %v", e)
	}
	defer setupClose.Call(set)
	var devices []Device
	for i := uint32(0); ; i++ {
		data := deviceInterfaceData{Size: uint32(unsafe.Sizeof(deviceInterfaceData{}))}
		ok, _, _ := setupEnum.Call(set, 0, uintptr(unsafe.Pointer(&guid)), uintptr(i), uintptr(unsafe.Pointer(&data)))
		if ok == 0 {
			break
		}
		var required uint32
		setupDetail.Call(set, uintptr(unsafe.Pointer(&data)), 0, 0, uintptr(unsafe.Pointer(&required)), 0)
		if required < 8 || required > 65536 {
			continue
		}
		buf := make([]byte, required)
		cbSize := uint32(8)
		if unsafe.Sizeof(uintptr(0)) == 4 {
			cbSize = 6
		}
		*(*uint32)(unsafe.Pointer(&buf[0])) = cbSize
		ok, _, _ = setupDetail.Call(set, uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&buf[0])), uintptr(required), uintptr(unsafe.Pointer(&required)), 0)
		if ok == 0 {
			continue
		}
		path := syscall.UTF16ToString((*[1 << 15]uint16)(unsafe.Pointer(&buf[4]))[:(len(buf)-4)/2])
		name, manufacturer, knxDevice := knxHIDStrings(path)
		if !knxDevice {
			continue
		}
		if name == "" {
			name = "KNX USB"
		}
		devices = append(devices, Device{Path: path, Name: name, Manufacturer: manufacturer})
	}
	return devices, nil
}

func knxHIDStrings(path string) (name, manufacturer string, ok bool) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", "", false
	}
	h, err := syscall.CreateFile(p, 0, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		return "", "", false
	}
	defer syscall.CloseHandle(h)
	var parsed uintptr
	result, _, _ := hidPreparsed.Call(uintptr(h), uintptr(unsafe.Pointer(&parsed)))
	if result == 0 {
		return "", "", false
	}
	defer hidFree.Call(parsed)
	var caps [64]byte
	status, _, _ := hidCaps.Call(parsed, uintptr(unsafe.Pointer(&caps[0])))
	if int32(status) < 0 {
		return "", "", false
	}
	usage := *(*uint16)(unsafe.Pointer(&caps[0]))
	page := *(*uint16)(unsafe.Pointer(&caps[2]))
	if page != 0xffa0 || usage != 1 {
		return "", "", false
	}
	return hidString(h, hidProduct), hidString(h, hidMaker), true
}

func hidString(handle syscall.Handle, proc *syscall.LazyProc) string {
	var chars [128]uint16
	ok, _, _ := proc.Call(uintptr(handle), uintptr(unsafe.Pointer(&chars[0])), uintptr(unsafe.Sizeof(chars)))
	if ok == 0 {
		return ""
	}
	return syscall.UTF16ToString(chars[:])
}
