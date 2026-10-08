//go:build linux

package knx

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
)

func listUSBDevices() ([]USBDevice, error) {
	paths, err := filepath.Glob("/sys/class/hidraw/hidraw*/device/report_descriptor")
	if err != nil {
		return nil, err
	}
	var devices []USBDevice
	for _, path := range paths {
		desc, err := os.ReadFile(path)
		if err != nil || !bytes.Contains(desc, []byte{0x06, 0xa0, 0xff, 0x09, 0x01}) {
			continue
		}
		name := filepath.Base(filepath.Dir(filepath.Dir(path)))
		devices = append(devices, USBDevice{Path: filepath.Join("/dev", name), Name: name})
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].Path < devices[j].Path })
	return devices, nil
}
