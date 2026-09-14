// Licensed under the MIT license which can be found in the LICENSE file.

package knx

import (
	"fmt"
	"net"
	"time"

	"github.com/vapourismo/knx-go/knx/knxnet"
	"golang.org/x/net/ipv4"
)

// Discover all KNXnet/IP servers.
func Discover(multicastDiscoveryAddress string, searchTimeout time.Duration) ([]*knxnet.SearchRes, error) {
	return DiscoverOnInterface(nil, multicastDiscoveryAddress, searchTimeout)
}

// interfaceIPv4 返回网卡的 IPv4 地址,无则返回 nil。
func interfaceIPv4(ifi *net.Interface) net.IP {
	addrs, err := ifi.Addrs()
	if err != nil {
		return nil
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP.To4() != nil {
			return ipNet.IP.To4()
		}
	}
	return nil
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

	// 指定网卡:bind 该网卡 IPv4 + 随机端口,并强制组播从该网卡发出。
	// 原来 ListenRouterOnInterface 只 JoinGroup(接收侧指定网卡),发送仍走系统默认组播
	// 路由接口,多网卡设备上 SEARCH_REQUEST 永远只从默认网卡出去,导致漏发现。
	ifaceIP := interfaceIPv4(ifi)
	if ifaceIP == nil {
		return nil, fmt.Errorf("knx discover: interface %s has no IPv4 address", ifi.Name)
	}

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: ifaceIP, Port: 0})
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	pc := ipv4.NewPacketConn(conn)
	if err := pc.SetMulticastInterface(ifi); err != nil {
		return nil, err
	}

	group, err := net.ResolveUDPAddr("udp4", multicastDiscoveryAddress)
	if err != nil {
		return nil, err
	}

	// discovery endpoint 填 网卡IP:本地端口,路由器把 SEARCH_RES 单播回这个 socket
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
				break // 等待超时,正常结束
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
