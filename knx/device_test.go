package knx

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/vapourismo/knx-go/knx/cemi"
)

type deviceFixture struct {
	address     cemi.IndividualAddr
	in          chan cemi.Message
	mu          sync.Mutex
	closed      bool
	connects    int
	acks        int
	lastWrite   []byte
	rejectWrite bool
}

func TestGroupTunnelSharesInboundWithDevice(t *testing.T) {
	raw := make(chan cemi.Message)
	gt := GroupTunnel{Tunnel: &Tunnel{inbound: raw}, inbound: make(chan GroupEvent), device: &groupTunnelDeviceState{}}
	go gt.serveInbound()
	address := cemi.NewIndividualAddr3(1, 1, 10)
	device, err := gt.OpenDeviceConnectionless(address)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gt.OpenDeviceConnectionless(address); err == nil {
		t.Fatal("second device session should be rejected")
	}
	raw <- &cemi.LDataInd{LData: cemi.LData{Source: address, Data: &cemi.AppData{Command: cemi.APCI(apciDeviceDescriptorResponse >> 6), Data: []byte{byte(apciDeviceDescriptorResponse & 0x3f), 0x12, 0x34}}}}
	raw <- &cemi.LDataInd{LData: cemi.LData{Source: address, Control2: cemi.Control2GroupAddr, Destination: 0x1234, Data: &cemi.AppData{Command: cemi.APCI(GroupWrite), Data: []byte{0x55}}}}
	select {
	case frame := <-device.inbound:
		if frame.Source != address {
			t.Fatalf("device frame source = %s", frame.Source)
		}
	case <-time.After(time.Second):
		t.Fatal("device did not receive unicast response")
	}
	select {
	case event := <-gt.Inbound():
		if event.Destination != 0x1234 || event.Command != GroupWrite {
			t.Fatalf("wrong group event: %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("group monitor did not receive event")
	}
	device.Close()
	if _, err := gt.OpenDeviceConnectionless(address); err != nil {
		t.Fatalf("reopen device: %v", err)
	}
	close(raw)
}

func (f *deviceFixture) Inbound() <-chan cemi.Message { return f.in }
func (f *deviceFixture) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.closed {
		f.closed = true
		close(f.in)
	}
}
func (f *deviceFixture) Send(msg cemi.Message) error {
	req := msg.(*cemi.LDataReq)
	switch unit := req.Data.(type) {
	case *cemi.ControlData:
		f.mu.Lock()
		if unit.Command == 0 && !unit.Numbered {
			f.connects++
		}
		if unit.Command == 2 && unit.Numbered {
			f.acks++
		}
		f.mu.Unlock()
	case *cemi.AppData:
		var responseCode uint16
		var response []byte
		switch apci(unit) {
		case apciDeviceDescriptorRead:
			responseCode = apciDeviceDescriptorResponse
			response = []byte{0x12, 0x34}
		case apciPropertyValueRead:
			responseCode = apciPropertyValueResponse
			request := unit.Data[1:]
			response = append([]byte(nil), request...)
			if request[1] == PIDIOList && request[2] == 0x10 && request[3] == 0 {
				response = append(response, 0, 2)
			}
			if request[1] == PIDIOList && request[2] == 0x10 && request[3] == 1 {
				response = append(response, 0, 0)
			}
			if request[1] == PIDIOList && request[2] == 0x10 && request[3] == 2 {
				response = append(response, 0, 11)
			}
		case apciPropertyValueWrite:
			responseCode = apciPropertyValueResponse
			f.mu.Lock()
			f.lastWrite = append([]byte(nil), unit.Data[1:]...)
			reject := f.rejectWrite
			f.mu.Unlock()
			response = append([]byte(nil), unit.Data[1:5]...)
			if reject {
				response[2] &= 0x0f
			}
		case apciPropertyDescriptionRead:
			responseCode = apciPropertyDescriptionResponse
			response = append(append([]byte(nil), unit.Data[1:]...), 0x84, 0x00, 0x20, 0x31)
		default:
			return nil
		}
		app := &cemi.AppData{Numbered: unit.Numbered, SeqNumber: unit.SeqNumber, Command: cemi.APCI(responseCode >> 6), Data: append([]byte{byte(responseCode & 0x3f)}, response...)}
		f.in <- &cemi.LDataInd{LData: cemi.LData{Source: f.address, Data: app}}
	}
	return nil
}

func TestDeviceWritePropertyValue(t *testing.T) {
	f := &deviceFixture{address: cemi.NewIndividualAddr3(1, 1, 10), in: make(chan cemi.Message, 16)}
	defer f.Close()
	device, err := openDevice(context.Background(), f, 0, f.address, true)
	if err != nil {
		t.Fatal(err)
	}
	defer device.Close()

	if err := device.WritePropertyValue(context.Background(), 1, 42, 2, 0x123, []byte{0xab, 0xcd}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	got := append([]byte(nil), f.lastWrite...)
	f.rejectWrite = true
	f.mu.Unlock()
	want := []byte{1, 42, 0x21, 0x23, 0xab, 0xcd}
	if !bytes.Equal(got, want) {
		t.Fatalf("write payload = %x, want %x", got, want)
	}
	if err := device.WritePropertyValue(context.Background(), 1, 42, 1, 1, []byte{0x01}); err == nil {
		t.Fatal("device rejection should fail")
	}
	for _, tc := range []struct {
		count uint8
		start uint16
		value []byte
	}{
		{0, 1, []byte{1}}, {16, 1, []byte{1}}, {1, 0, []byte{1}},
		{1, 0x1000, []byte{1}}, {1, 1, nil}, {1, 1, make([]byte, 251)},
	} {
		if err := device.WritePropertyValue(context.Background(), 1, 42, tc.count, tc.start, tc.value); err == nil {
			t.Fatalf("invalid write accepted: count=%d start=%d length=%d", tc.count, tc.start, len(tc.value))
		}
	}
}

func TestDeviceReadInterfaceObjectsAndPropertyDescription(t *testing.T) {
	f := &deviceFixture{address: cemi.NewIndividualAddr3(1, 1, 10), in: make(chan cemi.Message, 16)}
	defer f.Close()
	device, err := openDevice(context.Background(), f, 0, f.address, true)
	if err != nil {
		t.Fatal(err)
	}
	defer device.Close()
	objects, err := device.ReadInterfaceObjects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 2 || objects[0].Index != 0 || objects[0].Type != 0 || objects[1].Index != 1 || objects[1].Type != 11 {
		t.Fatalf("wrong objects: %+v", objects)
	}
	desc, err := device.ReadPropertyDescriptionByID(context.Background(), 0, PIDIOList)
	if err != nil {
		t.Fatal(err)
	}
	if !desc.WriteEnabled || desc.PropertyDataType != 4 || desc.MaxElements != 32 || desc.ReadLevel != 3 || desc.WriteLevel != 1 {
		t.Fatalf("wrong property description: %+v", desc)
	}
	f.mu.Lock()
	connects, acks := f.connects, f.acks
	f.mu.Unlock()
	if connects != 1 || acks != 5 {
		t.Fatalf("connects=%d acks=%d", connects, acks)
	}
}
