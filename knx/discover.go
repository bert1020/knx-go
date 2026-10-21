// Licensed under the MIT license which can be found in the LICENSE file.

package knx

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/vapourismo/knx-go/knx/knxnet"
)

// InterfaceKind identifies the transport supported by a discovered interface.
type InterfaceKind string

const (
	InterfaceIPTunneling InterfaceKind = "ip-tunneling"
	InterfaceIPRouting   InterfaceKind = "ip-routing"
	InterfaceUSB         InterfaceKind = "usb"
)

// DiscoveredInterface is an IP or USB connection candidate. Address is a
// gateway host:port, routing multicast host:port, or operating-system HID path.
type DiscoveredInterface struct {
	Kind    InterfaceKind
	Address string
	Name    string
	IP      *knxnet.SearchRes
	USB     *USBDevice
}

// DiscoverInterfaces combines the existing KNXnet/IP discovery with KNX USB
// HID enumeration. Discover and DiscoverOnInterface retain their IP-only API
// and return types for callers that already use them.
func DiscoverInterfaces(multicastDiscoveryAddress string, searchTimeout time.Duration) ([]DiscoveredInterface, error) {
	return DiscoverInterfacesOnInterface(nil, multicastDiscoveryAddress, searchTimeout)
}

// DiscoverInterfacesOnInterface uses the existing IP discovery on ifi and also
// enumerates local USB HID interfaces. Both run concurrently, since each one
// waits for its own timeout.
func DiscoverInterfacesOnInterface(ifi *net.Interface, multicastDiscoveryAddress string, searchTimeout time.Duration) ([]DiscoveredInterface, error) {
	var (
		wg     sync.WaitGroup
		ip     []*knxnet.SearchRes
		usb    []USBDevice
		ipErr  error
		usbErr error
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		ip, ipErr = DiscoverOnInterface(ifi, multicastDiscoveryAddress, searchTimeout)
	}()
	go func() {
		defer wg.Done()
		usb, usbErr = ListUSBDevices()
		if errors.Is(usbErr, ErrUSBUnsupported) {
			usbErr = nil
		}
	}()
	wg.Wait()
	result := combineDiscoveredInterfaces(ip, usb)
	if ipErr != nil && usbErr != nil {
		return result, fmt.Errorf("IP discovery: %v; USB discovery: %v", ipErr, usbErr)
	}
	if ipErr != nil {
		return result, fmt.Errorf("IP discovery: %w", ipErr)
	}
	if usbErr != nil {
		return result, fmt.Errorf("USB discovery: %w", usbErr)
	}
	return result, nil
}

func combineDiscoveredInterfaces(ip []*knxnet.SearchRes, usb []USBDevice) []DiscoveredInterface {
	result := make([]DiscoveredInterface, 0, len(ip)+len(usb))
	seen := make(map[string]bool)
	for _, response := range ip {
		if response == nil {
			continue
		}
		host := net.JoinHostPort(response.Control.Address.String(), strconv.Itoa(int(response.Control.Port)))
		name := response.DescriptionB.DeviceHardware.FriendlyName
		for _, service := range response.DescriptionB.SupportedServices.Families {
			var kind InterfaceKind
			var address string
			switch service.Type {
			case knxnet.ServiceFamilyTypeIPTunnelling:
				kind, address = InterfaceIPTunneling, host
			case knxnet.ServiceFamilyTypeIPRouting:
				multicast := response.DescriptionB.DeviceHardware.RoutingMulticastAddress.String()
				if multicast == "0.0.0.0" {
					multicast = "224.0.23.12"
				}
				kind, address = InterfaceIPRouting, net.JoinHostPort(multicast, "3671")
			default:
				continue
			}
			key := string(kind) + "|" + host
			if seen[key] {
				continue
			}
			seen[key] = true
			result = append(result, DiscoveredInterface{Kind: kind, Address: address, Name: name, IP: response})
		}
	}
	for i := range usb {
		device := usb[i]
		if device.Path == "" {
			continue
		}
		key := string(InterfaceUSB) + "|" + device.Path
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, DiscoveredInterface{Kind: InterfaceUSB, Address: device.Path, Name: device.Name, USB: &device})
	}
	return result
}

// Discover all KNXnet/IP servers.
func Discover(multicastDiscoveryAddress string, searchTimeout time.Duration) ([]*knxnet.SearchRes, error) {
	return DiscoverOnInterface(nil, multicastDiscoveryAddress, searchTimeout)
}

// interfaceIPv4 返回网卡的第一个 IPv4 地址,无则返回 nil。
func interfaceIPv4(ifi *net.Interface) net.IP {
	ips := interfaceIPv4s(ifi)
	if len(ips) == 0 {
		return nil
	}
	return ips[0]
}

// interfaceIPv4s 返回网卡上全部可用于发现的 IPv4。
// 同一张网卡配了多个地址时（例如 192.168.2.188 和 192.168.123.188），
// 只从第一个地址发 SEARCH_REQUEST 会漏掉其它网段上的网关。
func interfaceIPv4s(ifi *net.Interface) []net.IP {
	if ifi == nil {
		return nil
	}
	addrs, err := ifi.Addrs()
	if err != nil {
		return nil
	}
	seen := map[string]struct{}{}
	var all, unicast []net.IP
	for _, addr := range addrs {
		var ip net.IP
		switch a := addr.(type) {
		case *net.IPNet:
			ip = a.IP.To4()
		case *net.IPAddr:
			ip = a.IP.To4()
		}
		if ip == nil {
			continue
		}
		ip = append(net.IP(nil), ip...)
		key := ip.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		all = append(all, ip)
		if ip.IsGlobalUnicast() {
			unicast = append(unicast, ip)
		}
	}
	if len(unicast) > 0 {
		return unicast
	}
	return all
}

// DiscoverOnInterface discovers all KNXnet/IP servers on a specific interface. If the
// interface is nil, the system-assigned multicast interface is used.
func DiscoverOnInterface(ifi *net.Interface, multicastDiscoveryAddress string, searchTimeout time.Duration) ([]*knxnet.SearchRes, error) {
	// 未指定网卡:保持原行为,bind 组播地址 + 系统默认组播接口
	if ifi == nil {
		socket, err := knxnet.ListenRouterOnInterface(nil, multicastDiscoveryAddress, false)
		if err != nil {
			return nil, err
		}
		defer socket.Close()

		req, err := knxnet.NewSearchReq(socket.Addr())
		if err != nil {
			return nil, err
		}

		if err := socket.Send(req); err != nil {
			return nil, err
		}

		results := []*knxnet.SearchRes{}
		timeout := time.After(searchTimeout)

	loop:
		for {
			select {
			case msg := <-socket.Inbound():
				searchRes, ok := msg.(*knxnet.SearchRes)
				if !ok {
					continue
				}
				results = append(results, searchRes)

			case <-timeout:
				break loop
			}
		}

		return results, nil
	}

	// 指定网卡:每个 IPv4 各绑一个 socket 并发搜索。
	// Windows 上 IP_MULTICAST_IF 按地址而不是网卡索引生效，只设网卡会固定用第一个
	// 地址当组播源。网关在第二个地址的网段（如 192.168.123.200）时，SEARCH_RES
	// 回不到第一个地址，结果就被漏掉。
	ips := interfaceIPv4s(ifi)
	if len(ips) == 0 {
		return nil, fmt.Errorf("knx discover: interface %s has no IPv4 address", ifi.Name)
	}
	if len(ips) == 1 {
		return discoverOnIPv4(ips[0], multicastDiscoveryAddress, searchTimeout)
	}

	type searchResult struct {
		res []*knxnet.SearchRes
		err error
	}
	ch := make(chan searchResult, len(ips))
	var wg sync.WaitGroup
	for _, ip := range ips {
		wg.Add(1)
		go func(ip net.IP) {
			defer wg.Done()
			res, err := discoverOnIPv4(ip, multicastDiscoveryAddress, searchTimeout)
			ch <- searchResult{res, err}
		}(ip)
	}
	wg.Wait()
	close(ch)

	var results []*knxnet.SearchRes
	var firstErr error
	for r := range ch {
		if r.err != nil {
			if firstErr == nil {
				firstErr = r.err
			}
			continue
		}
		results = append(results, r.res...)
	}
	if len(results) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return results, nil
}

// discoverOnIPv4 从指定本机地址发出 SEARCH_REQUEST，并在同一 socket 上收单播响应。
func discoverOnIPv4(ifaceIP net.IP, multicastDiscoveryAddress string, searchTimeout time.Duration) ([]*knxnet.SearchRes, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: ifaceIP, Port: 0})
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := setMulticastSource(conn, ifaceIP); err != nil {
		return nil, err
	}

	group, err := net.ResolveUDPAddr("udp4", multicastDiscoveryAddress)
	if err != nil {
		return nil, err
	}

	// discovery endpoint 填本机地址:本地端口,路由器把 SEARCH_RES 单播回这个 socket
	local := conn.LocalAddr().(*net.UDPAddr)
	req, err := knxnet.NewSearchReq(&net.UDPAddr{IP: ifaceIP, Port: local.Port})
	if err != nil {
		return nil, err
	}

	if _, err := conn.WriteTo(knxnet.AllocAndPack(req), group); err != nil {
		return nil, err
	}

	results := []*knxnet.SearchRes{}
	buf := make([]byte, 1024)
	deadline := time.Now().Add(searchTimeout)
	for {
		conn.SetReadDeadline(deadline)
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				break
			}
			return results, err
		}
		var srv knxnet.Service
		if _, err := knxnet.Unpack(buf[:n], &srv); err != nil {
			continue
		}
		if res, ok := srv.(*knxnet.SearchRes); ok {
			results = append(results, res)
		}
	}

	return results, nil
}
