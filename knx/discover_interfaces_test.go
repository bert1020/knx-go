package knx

import (
	"testing"

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
