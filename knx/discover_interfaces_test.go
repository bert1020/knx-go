package knx

import (
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/vapourismo/knx-go/knx/knxnet"
)

func TestCombineDiscoveredInterfaces(t *testing.T) {
	response := &knxnet.SearchRes{}
	response.Control.Address = knxnet.Address{192, 168, 1, 10}
	response.Control.Port = 3671
	response.DescriptionB.DeviceHardware.FriendlyName = "KNX IP"
	response.DescriptionB.DeviceHardware.RoutingMulticastAddress = knxnet.Address{224, 0, 23, 12}
	response.DescriptionB.SupportedServices.Families = []knxnet.ServiceFamily{
		{Type: knxnet.ServiceFamilyTypeIPTunnelling, Version: 1},
		{Type: knxnet.ServiceFamilyTypeIPRouting, Version: 1},
	}
	got := combineDiscoveredInterfaces([]*knxnet.SearchRes{response}, []USBDevice{{Path: "/dev/hidraw0", Name: "KNX USB"}})
	if len(got) != 3 {
		t.Fatalf("got %d interfaces", len(got))
	}
	if got[0].Kind != InterfaceIPTunneling || got[0].Address != "192.168.1.10:3671" {
		t.Fatalf("tunnel: %+v", got[0])
	}
	if got[1].Kind != InterfaceIPRouting || got[1].Address != "224.0.23.12:3671" {
		t.Fatalf("router: %+v", got[1])
	}
	if got[2].Kind != InterfaceUSB || got[2].Address != "/dev/hidraw0" {
		t.Fatalf("USB: %+v", got[2])
	}
}

// Run with: KNX_DISCOVER_LIVE=1 go test ./knx -run '^TestDiscoverInterfacesLive$' -v
func TestDiscoverInterfacesLive(t *testing.T) {
	if os.Getenv("KNX_DISCOVER_LIVE") != "1" {
		t.Skip("set KNX_DISCOVER_LIVE=1 to scan local KNX interfaces")
	}
	const multicast = "224.0.23.12:3671"
	const timeout = 2 * time.Second
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	found := 0
	scan := func(ifi *net.Interface) {
		devices, err := DiscoverInterfacesOnInterface(ifi, multicast, timeout)
		if err != nil {
			if ifi == nil {
				t.Logf("default interface: %v", err)
			} else {
				t.Logf("interface %s: %v", ifi.Name, err)
			}
		}
		for _, device := range devices {
			if device.Kind != InterfaceIPTunneling && device.Kind != InterfaceUSB {
				continue
			}
			key := string(device.Kind) + "|" + device.Address
			if seen[key] {
				continue
			}
			seen[key] = true
			found++
			t.Logf("%s: %s (%s)", device.Kind, device.Name, device.Address)
		}
	}
	scanned := 0
	for i := range ifaces {
		ifi := &ifaces[i]
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 || interfaceIPv4(ifi) == nil {
			continue
		}
		scan(ifi)
		scanned++
	}
	if scanned == 0 {
		scan(nil)
	}
	if _, err := ListUSBDevices(); errors.Is(err, ErrUSBUnsupported) {
		t.Log("USB discovery is unsupported on this operating system")
	}
	if found == 0 {
		t.Log("no IP tunneling or USB interfaces found")
	}
}
