package knx

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/vapourismo/knx-go/knx/cemi"
)

// USBDevice identifies a KNX USB HID interface. Path is passed to NewUSB.
type USBDevice struct{ Path, Name string }

// ErrUSBUnsupported indicates that this operating system has no USB HID backend.
var ErrUSBUnsupported = errors.New("KNX USB HID is currently supported on Windows and Linux")

// ListUSBDevices discovers interfaces with the KNX HID usage page.
func ListUSBDevices() ([]USBDevice, error) { return listUSBDevices() }

// USB provides the same cEMI Send/Inbound interface as Tunnel and Router.
// It supports cEMI-capable KNX USB HID interfaces on Windows and Linux.
type USB struct {
	file    *os.File
	inbound chan cemi.Message
	done    chan struct{}
	once    sync.Once
	writeMu sync.Mutex
}

// NewUSB opens a KNX USB HID interface and negotiates cEMI mode.
func NewUSB(ctx context.Context, path string) (*USB, error) {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		return nil, ErrUSBUnsupported
	}
	if path == "" {
		return nil, errors.New("USB HID path is empty")
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open KNX USB HID: %w", err)
	}
	u := &USB{file: f, inbound: make(chan cemi.Message, 64), done: make(chan struct{})}
	if err := u.write(0x0f, 0x01, []byte{0x01}); err != nil {
		f.Close()
		return nil, err
	}
	ready := make(chan error, 1)
	go u.read(ready)
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case err := <-ready:
		if err != nil {
			u.Close()
			return nil, fmt.Errorf("KNX USB cEMI negotiation: %w", err)
		}
	case <-ctx.Done():
		u.Close()
		return nil, ctx.Err()
	case <-timer.C:
		u.Close()
		return nil, errors.New("KNX USB cEMI capability query timed out")
	}
	return u, nil
}

func (u *USB) write(protocol, service byte, body []byte) error {
	reports, err := usbEncode(protocol, service, body)
	if err != nil {
		return err
	}
	u.writeMu.Lock()
	defer u.writeMu.Unlock()
	for _, report := range reports {
		if _, err := u.file.Write(report); err != nil {
			return err
		}
	}
	return nil
}

func (u *USB) read(ready chan<- error) {
	defer close(u.inbound)
	var assembler usbAssembler
	negotiated := false
	for {
		report := make([]byte, usbReportSize)
		_, err := io.ReadFull(u.file, report)
		if err != nil {
			if !negotiated {
				ready <- err
			}
			return
		}
		packet, complete, err := assembler.Add(report)
		if err != nil || !complete {
			continue
		}
		if !negotiated && packet.protocol == 0x0f && packet.service == 0x02 && len(packet.body) >= 3 && packet.body[0] == 0x01 {
			if binary.BigEndian.Uint16(packet.body[1:3])&4 == 0 {
				ready <- errors.New("USB interface does not support cEMI")
				return
			}
			if err := u.write(0x0f, 0x03, []byte{0x05, 0x03}); err != nil {
				ready <- err
				return
			}
			negotiated = true
			ready <- nil
			continue
		}
		if packet.protocol != 1 || packet.service != 3 || len(packet.body) == 0 {
			continue
		}
		var msg cemi.Message
		if _, err := safeUSBUnpack(packet.body, &msg); err != nil {
			continue
		}
		select {
		case u.inbound <- msg:
		case <-u.done:
			return
		}
	}
}

func safeUSBUnpack(data []byte, out *cemi.Message) (n uint, err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("malformed USB cEMI frame")
		}
	}()
	n, err = cemi.Unpack(data, out)
	if err == nil && int(n) != len(data) {
		err = errors.New("trailing USB cEMI data")
	}
	return n, err
}

func (u *USB) Send(msg cemi.Message) error {
	if msg == nil {
		return errors.New("nil cEMI message")
	}
	select {
	case <-u.done:
		return errors.New("USB is closed")
	default:
	}
	frame := make([]byte, cemi.Size(msg))
	cemi.Pack(frame, msg)
	return u.write(1, 3, frame)
}

func (u *USB) Inbound() <-chan cemi.Message { return u.inbound }

func (u *USB) Close() {
	u.once.Do(func() { close(u.done); u.file.Close() })
}

// GroupUSB exposes group telegrams using the same API as GroupTunnel.
type GroupUSB struct {
	*USB
	inbound chan GroupEvent
}

func NewGroupUSB(ctx context.Context, path string) (*GroupUSB, error) {
	u, err := NewUSB(ctx, path)
	if err != nil {
		return nil, err
	}
	g := &GroupUSB{USB: u, inbound: make(chan GroupEvent)}
	go serveGroupInbound(u.Inbound(), g.inbound)
	return g, nil
}

func (g *GroupUSB) Send(event GroupEvent) error {
	return g.USB.Send(&cemi.LDataReq{LData: buildGroupOutbound(event)})
}

func (g *GroupUSB) Inbound() <-chan GroupEvent { return g.inbound }
