package knx

import (
	"fmt"
	"github.com/vapourismo/knx-go/knx/knxnet"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDiscover(t *testing.T) {
	routers, err := DiscoverKnxRouters(2 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("routers:", routers)
}

const (
	MulticastAddress = "224.0.23.12:3671"
)

// KnxRouterInfo 搜索到的 KNX IP Router 信息
type KnxRouterInfo struct {
	IpAddr string `json:"ipAddr"`
	Port   int    `json:"port"`
	Name   string `json:"name"`
}

// discoverExcludePrefixes 发现时排除的回环/虚拟网卡名前缀。
// 注意不排除 WLAN 等无线物理网卡——多网卡设备上无线也要发发现请求。
var discoverExcludePrefixes = []string{
	"lo", "docker", "veth", "br-", "virbr", "tun", "tap",
	"vEthernet", "Virtual", "VMware",
}

// isDiscoverableInterface 判断网卡是否可用于组播发现:
// Up 状态、非回环、有 MAC、有非回环 IPv4、非虚拟网卡。
func isDiscoverableInterface(iface net.Interface) bool {
	if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
		return false
	}
	if len(iface.HardwareAddr) == 0 {
		return false
	}
	name := iface.Name
	for _, p := range discoverExcludePrefixes {
		if strings.HasPrefix(name, p) {
			return false
		}
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
			return true
		}
	}
	return false
}

// appendDiscoverResult 合并搜索响应,按 IP:Port 去重
func appendDiscoverResult(result []KnxRouterInfo, seen map[string]bool, responses []*knxnet.SearchRes) []KnxRouterInfo {
	for _, res := range responses {
		key := fmt.Sprintf("%s:%d", res.Control.Address.String(), res.Control.Port)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, KnxRouterInfo{
			IpAddr: res.Control.Address.String(),
			Port:   int(res.Control.Port),
			Name:   res.DescriptionB.DeviceHardware.FriendlyName,
		})
	}
	return result
}

// DiscoverKnxRouters 通过 KNXnet/IP 组播搜索局域网内的 KNX IP Router。
// 系统默认组播路由只选一张网卡,多网卡设备上有时只走有线有时只走无线导致漏发现,
// 因此遍历除回环/虚拟外的所有网卡并发发送 SEARCH_REQUEST,结果按 IP:Port 去重;
// 无可用网卡时回退系统默认组播接口。
func DiscoverKnxRouters(timeout time.Duration) ([]KnxRouterInfo, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	targets := make([]*net.Interface, 0, len(ifaces))
	for i := range ifaces {
		if isDiscoverableInterface(ifaces[i]) {
			targets = append(targets, &ifaces[i])
		}
	}
	if len(targets) == 0 {
		responses, err := Discover(MulticastAddress, timeout)
		return appendDiscoverResult(nil, make(map[string]bool), responses), err
	}

	type discResult struct {
		responses []*knxnet.SearchRes
	}
	ch := make(chan discResult, len(targets))
	var wg sync.WaitGroup
	for _, ifi := range targets {
		wg.Add(1)
		go func(ifi *net.Interface) {
			defer wg.Done()
			responses, err := DiscoverOnInterface(ifi, MulticastAddress, timeout)
			if err != nil {
				return
			}
			ch <- discResult{responses: responses}
		}(ifi)
	}
	wg.Wait()
	close(ch)

	seen := make(map[string]bool)
	var result []KnxRouterInfo
	for r := range ch {
		result = appendDiscoverResult(result, seen, r.responses)
	}
	return result, nil
}
