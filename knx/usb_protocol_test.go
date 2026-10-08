package knx

import (
	"bytes"
	"testing"

	"github.com/vapourismo/knx-go/knx/cemi"
)

func TestUSBTransferRoundTrip(t *testing.T) {
	for _, size := range []int{0, 1, 53, 54, 100, 263} {
		body := bytes.Repeat([]byte{0x29}, size)
		reports, err := usbEncode(1, 3, body)
		if err != nil {
			t.Fatal(err)
		}
		var decoder usbAssembler
		for i, report := range reports {
			got, done, err := decoder.Add(report)
			if err != nil {
				t.Fatal(err)
			}
			if done != (i == len(reports)-1) {
				t.Fatalf("size %d fragment %d: done=%v", size, i, done)
			}
			if done && (got.protocol != 1 || got.service != 3 || !bytes.Equal(got.body, body)) {
				t.Fatalf("size %d: wrong transfer", size)
			}
		}
	}
}

func TestUSBRejectsBadContinuation(t *testing.T) {
	reports, err := usbEncode(1, 3, bytes.Repeat([]byte{1}, 100))
	if err != nil {
		t.Fatal(err)
	}
	var decoder usbAssembler
	_, _, _ = decoder.Add(reports[0])
	reports[1][1] = 0x34
	if _, _, err := decoder.Add(reports[1]); err == nil {
		t.Fatal("accepted wrong sequence")
	}
}

func TestUSBGroupCEMIFrame(t *testing.T) {
	address := cemi.NewGroupAddr3(1, 2, 3)
	ldata := buildGroupOutbound(GroupEvent{Command: GroupWrite, Destination: address, Data: []byte{1}})
	ldata.Source = cemi.NewIndividualAddr3(1, 1, 10)
	message := &cemi.LDataInd{LData: ldata}
	frame := make([]byte, cemi.Size(message))
	cemi.Pack(frame, message)
	reports, err := usbEncode(1, 3, frame)
	if err != nil {
		t.Fatal(err)
	}
	var assembler usbAssembler
	var packet usbPacket
	for _, report := range reports {
		packet, _, err = assembler.Add(report)
		if err != nil {
			t.Fatal(err)
		}
	}
	var decoded cemi.Message
	if _, err := safeUSBUnpack(packet.body, &decoded); err != nil {
		t.Fatal(err)
	}
	ind, ok := decoded.(*cemi.LDataInd)
	if !ok || ind.Source != ldata.Source || ind.Destination != uint16(address) {
		t.Fatalf("wrong cEMI frame: %#v", decoded)
	}
	app, ok := ind.Data.(*cemi.AppData)
	if !ok || app.Command != cemi.GroupValueWrite || !bytes.Equal(app.Data, []byte{1}) {
		t.Fatalf("wrong group APDU: %#v", ind.Data)
	}
}
