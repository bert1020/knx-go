package knx

import (
	"context"
	"sync"
	"testing"

	"github.com/vapourismo/knx-go/knx/cemi"
)

type deviceFixture struct {
	address  cemi.IndividualAddr
	in       chan cemi.Message
	mu       sync.Mutex
	closed   bool
	connects int
	acks     int
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
