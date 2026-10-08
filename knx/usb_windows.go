//go:build windows

package knx

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	hidDLL       = syscall.NewLazyDLL("hid.dll")
	setupDLL     = syscall.NewLazyDLL("setupapi.dll")
	hidGuid      = hidDLL.NewProc("HidD_GetHidGuid")
	hidPreparsed = hidDLL.NewProc("HidD_GetPreparsedData")
	hidFree      = hidDLL.NewProc("HidD_FreePreparsedData")
	hidCaps      = hidDLL.NewProc("HidP_GetCaps")
	setupGet     = setupDLL.NewProc("SetupDiGetClassDevsW")
	setupEnum    = setupDLL.NewProc("SetupDiEnumDeviceInterfaces")
	setupDetail  = setupDLL.NewProc("SetupDiGetDeviceInterfaceDetailW")
	setupClose   = setupDLL.NewProc("SetupDiDestroyDeviceInfoList")
)

type deviceInterfaceData struct {
	Size      uint32
	ClassGUID syscall.GUID
	Flags     uint32
	Reserved  uintptr
}

func listUSBDevices() ([]USBDevice, error) {
	var guid syscall.GUID
	hidGuid.Call(uintptr(unsafe.Pointer(&guid)))
	set, _, e := setupGet.Call(uintptr(unsafe.Pointer(&guid)), 0, 0, 0x12) // PRESENT | DEVICEINTERFACE
	if set == ^uintptr(0) {
		return nil, fmt.Errorf("SetupDiGetClassDevsW: %v", e)
	}
	defer setupClose.Call(set)
	var devices []USBDevice
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
		if !isKNXHID(path) {
			continue
		}
		devices = append(devices, USBDevice{Path: path, Name: path})
	}
	return devices, nil
}

func isKNXHID(path string) bool {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	h, err := syscall.CreateFile(p, 0, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	var parsed uintptr
	ok, _, _ := hidPreparsed.Call(uintptr(h), uintptr(unsafe.Pointer(&parsed)))
	if ok == 0 {
		return false
	}
	defer hidFree.Call(parsed)
	var caps [64]byte
	status, _, _ := hidCaps.Call(parsed, uintptr(unsafe.Pointer(&caps[0])))
	if int32(status) < 0 {
		return false
	}
	usage := *(*uint16)(unsafe.Pointer(&caps[0]))
	page := *(*uint16)(unsafe.Pointer(&caps[2]))
	return page == 0xffa0 && usage == 1
}
