package knx

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"runtime"
	"sync"
	"time"

	"github.com/vapourismo/knx-go/knx/cemi"
	"github.com/vapourismo/knx-go/knx/knxusb"
)

// USBDevice identifies a KNX USB HID interface. Path is passed to NewUSB.
type USBDevice = knxusb.Device

// ErrUSBUnsupported indicates that this operating system has no USB HID backend.
var ErrUSBUnsupported = knxusb.ErrUnsupported

// Timed-out writes are retried at most this many times. A timed-out report
// was not delivered to the interface, so retrying it is safe.
const usbWriteAttempts = 8

// ListUSBDevices discovers KNX HID interfaces and reads local device details
// when the interface is available. Devices remain listed if probing fails.
func ListUSBDevices() ([]USBDevice, error) {
	devices, err := knxusb.ListDevices()
	if err != nil {
		return nil, err
	}
	for i := range devices {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		probeUSBDevice(ctx, &devices[i])
		cancel()
	}
	return devices, nil
}

// USB provides the same cEMI Send/Inbound interface as Tunnel and Router.
// Interfaces that speak cEMI are used directly. EMI1-only interfaces are
// switched to link-layer mode and their L_Data frames are translated to cEMI.
type USB struct {
	file    knxusb.Port
	inbound chan cemi.Message
	done    chan struct{}
	once    sync.Once
	writeMu sync.Mutex
	localMu sync.Mutex
	conMu   sync.Mutex
	conCh   chan error
	emi     byte // 0x01 EMI1 or 0x03 cEMI, set before NewUSB returns
}

// NewUSB opens a KNX USB HID interface and negotiates cEMI or EMI1 mode.
func NewUSB(ctx context.Context, path string) (*USB, error) {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		return nil, ErrUSBUnsupported
	}
	if path == "" {
		return nil, errors.New("USB HID path is empty")
	}
	port, err := knxusb.OpenPort(path)
	if err != nil {
		return nil, err
	}
	u := &USB{file: port, inbound: make(chan cemi.Message, 64), done: make(chan struct{})}
	if err := u.write(0x0f, 0x01, []byte{0x01}); err != nil {
		port.Close()
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
	reports, err := knxusb.Encode(protocol, service, body)
	if err != nil {
		return err
	}
	u.writeMu.Lock()
	defer u.writeMu.Unlock()
	for _, report := range reports {
		for attempt := 1; ; attempt++ {
			select {
			case <-u.done:
				return errors.New("USB is closed")
			default:
			}
			if _, err := u.file.Write(report); err == nil {
				break
			} else if !errors.Is(err, knxusb.ErrWriteTimeout) || attempt >= usbWriteAttempts {
				return err
			}
		}
	}
	return nil
}

// selectEMI picks cEMI when the interface offers it, otherwise EMI1.
// EMI1 also needs PC_SET_VAL.req (address 0x0060, value 0x12) to enter link layer.
func (u *USB) selectEMI(mask uint16) error {
	switch {
	case mask&4 != 0:
		u.emi = 0x03
		return u.write(0x0f, 0x03, []byte{0x05, 0x03})
	case mask&1 != 0:
		u.emi = 0x01
		if err := u.write(0x0f, 0x03, []byte{0x05, 0x01}); err != nil {
			return err
		}
		return u.write(0x01, 0x01, []byte{0x46, 0x01, 0x00, 0x60, 0x12})
	default:
		return errors.New("USB interface supports neither cEMI nor EMI1")
	}
}

func (u *USB) read(ready chan<- error) {
	defer close(u.inbound)
	var assembler knxusb.Assembler
	negotiated := false
	for {
		report := make([]byte, knxusb.ReportSize)
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
		if !negotiated && packet.Protocol == 0x0f && packet.Service == 0x02 && len(packet.Body) >= 3 && packet.Body[0] == 0x01 {
			if err := u.selectEMI(binary.BigEndian.Uint16(packet.Body[1:3])); err != nil {
				ready <- err
				return
			}
			// EMI1 interfaces often accept PC_SET_VAL.req without sending PC_SET_VAL.con.
			negotiated = true
			ready <- nil
			continue
		}
		if !negotiated || packet.Protocol != 0x01 || packet.Service != u.emi || len(packet.Body) == 0 {
			continue
		}
		raw := packet.Body
		if u.emi == 0x01 {
			raw, err = emi1ToCEMI(packet.Body)
			if err != nil {
				continue
			}
		}
		var msg cemi.Message
		if _, err := safeUSBUnpack(raw, &msg); err != nil {
			continue
		}
		// ETS waits for L_Data.con before sending the next EMI1 frame.
		// Forwarding it would let the next request overlap this one and the
		// device answers with T_Disconnect.
		if u.emi == 0x01 {
			if con, ok := msg.(*cemi.LDataCon); ok {
				var conErr error
				if con.Control1&cemi.Control1HasError != 0 {
					conErr = errors.New("EMI1 L_Data.con reported an error")
				}
				u.finishCon(conErr)
				continue
			}
		}
		select {
		case u.inbound <- msg:
		case <-u.done:
			return
		}
	}
}

func (u *USB) finishCon(err error) {
	u.conMu.Lock()
	ch := u.conCh
	u.conCh = nil
	u.conMu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- err:
	default:
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
	if u.emi == 0x01 {
		emi, err := cemiToEMI1(frame)
		if err != nil {
			return err
		}
		ch := make(chan error, 1)
		u.conMu.Lock()
		u.conCh = ch
		u.conMu.Unlock()
		if err := u.write(0x01, 0x01, emi); err != nil {
			u.finishCon(nil)
			return err
		}
		select {
		case err := <-ch:
			return err
		case <-u.done:
			return errors.New("USB is closed")
		case <-time.After(2 * time.Second):
			u.conMu.Lock()
			if u.conCh == ch {
				u.conCh = nil
			}
			u.conMu.Unlock()
			return errors.New("EMI1 L_Data.con timed out")
		}
	}
	return u.write(0x01, 0x03, frame)
}

func (u *USB) Inbound() <-chan cemi.Message { return u.inbound }

func (u *USB) Close() {
	u.once.Do(func() {
		close(u.done)
		u.file.Cancel()
		u.file.Close()
	})
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
