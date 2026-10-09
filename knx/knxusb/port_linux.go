//go:build linux

package knxusb

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// OpenPort opens a Linux hidraw device with plain blocking file I/O.
func OpenPort(path string) (Port, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open KNX USB HID: %w", err)
	}
	return PortFile{f}, nil
}

// ListDevices enumerates hidraw devices whose report descriptor matches the
// KNX USB usage page.
func ListDevices() ([]Device, error) {
	paths, err := filepath.Glob("/sys/class/hidraw/hidraw*/device/report_descriptor")
	if err != nil {
		return nil, err
	}
	var devices []Device
	for _, path := range paths {
		desc, err := os.ReadFile(path)
		if err != nil || !bytes.Contains(desc, []byte{0x06, 0xa0, 0xff, 0x09, 0x01}) {
			continue
		}
		name := filepath.Base(filepath.Dir(filepath.Dir(path)))
		product, manufacturer := sysfsStrings(filepath.Dir(path))
		if product == "" {
			product = "KNX USB"
		}
		devices = append(devices, Device{Path: filepath.Join("/dev", name), Name: product, Manufacturer: manufacturer})
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].Path < devices[j].Path })
	return devices, nil
}

func sysfsStrings(path string) (product, manufacturer string) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", ""
	}
	for dir := resolved; dir != "/"; dir = filepath.Dir(dir) {
		productBytes, productErr := os.ReadFile(filepath.Join(dir, "product"))
		manufacturerBytes, manufacturerErr := os.ReadFile(filepath.Join(dir, "manufacturer"))
		if productErr == nil || manufacturerErr == nil {
			return strings.TrimSpace(string(productBytes)), strings.TrimSpace(string(manufacturerBytes))
		}
	}
	return "", ""
}
