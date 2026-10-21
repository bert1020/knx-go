package knx

import (
	"bytes"
	"testing"

	"github.com/vapourismo/knx-go/knx/cemi"
)

func TestEMI1LDataRoundTrip(t *testing.T) {
	msg := &cemi.LDataReq{LData: cemi.LData{
		Control1:    cemi.Control1StdFrame | cemi.Control1NoRepeat | cemi.Control1Prio(cemi.PrioLow),
		Control2:    cemi.Control2GroupAddr | cemi.Control2Hops(6),
		Source:      cemi.NewIndividualAddr3(1, 1, 10),
		Destination: uint16(cemi.NewGroupAddr3(1, 0, 0)),
		Data:        &cemi.AppData{Command: cemi.GroupValueWrite, Data: []byte{0x81}},
	}}
	frame := make([]byte, cemi.Size(msg))
	cemi.Pack(frame, msg)

	emi, err := cemiToEMI1(frame)
	if err != nil {
		t.Fatal(err)
	}
	if emi[0] != 0x11 || emi[6]&0x80 == 0 || emi[6]&0x0F == 0 {
		t.Fatalf("emi1 frame %#x", emi)
	}
	back, err := emi1ToCEMI(emi)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(frame, back) {
		t.Fatalf("cemi %#x\nback %#x", frame, back)
	}
	var decoded cemi.Message
	if _, err := cemi.Unpack(back, &decoded); err != nil {
		t.Fatal(err)
	}
	req, ok := decoded.(*cemi.LDataReq)
	if !ok || !req.Control2.IsGroupAddr() || req.Destination != msg.Destination {
		t.Fatalf("decoded %#v", decoded)
	}
}

func TestEMI1IndicationCode(t *testing.T) {
	// Captured from the EMI1 USB interface: L_Data.ind, source 1.1.2.
	emi := []byte{0x49, 0xB0, 0x11, 0x02, 0x11, 0xE9, 0x60, 0x81}
	cemiFrame, err := emi1ToCEMI(emi)
	if err != nil {
		t.Fatal(err)
	}
	var msg cemi.Message
	if _, err := cemi.Unpack(cemiFrame, &msg); err != nil {
		t.Fatal(err)
	}
	ind, ok := msg.(*cemi.LDataInd)
	if !ok || ind.Source != cemi.NewIndividualAddr3(1, 1, 2) {
		t.Fatalf("%#v", msg)
	}
	ctrl, ok := ind.Data.(*cemi.ControlData)
	if !ok || ctrl.Command != 1 {
		t.Fatalf("tpdu %#v", ind.Data)
	}
}

func TestEMI1RejectsLocalPropertyRead(t *testing.T) {
	msg := &cemi.UnsupportedMessage{Code: 0xfc, Data: []byte{0, 0, 1, 12, 0x10, 1}}
	frame := make([]byte, cemi.Size(msg))
	cemi.Pack(frame, msg)
	if _, err := cemiToEMI1(frame); err == nil {
		t.Fatal("expected EMI1 to reject cEMI management frame")
	}
}
