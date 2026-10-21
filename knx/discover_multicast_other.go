//go:build !windows

package knx

import (
	"fmt"
	"net"
	"syscall"
)

// setMulticastSource 把 IPv4 组播出口地址设成这个 socket 绑定的地址。
func setMulticastSource(conn *net.UDPConn, ip net.IP) error {
	ip4 := ip.To4()
	if ip4 == nil {
		return fmt.Errorf("knx discover: %s is not IPv4", ip)
	}
	raw, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var addr [4]byte
	copy(addr[:], ip4)
	var sockErr error
	err = raw.Control(func(fd uintptr) {
		sockErr = syscall.SetsockoptInet4Addr(int(fd), syscall.IPPROTO_IP, syscall.IP_MULTICAST_IF, addr)
	})
	if err != nil {
		return err
	}
	return sockErr
}
