package knx

import (
	"bytes"
	"testing"

	"github.com/vapourismo/knx-go/knx/cemi"
	"github.com/vapourismo/knx-go/knx/knxusb"
)

func TestUSBGroupCEMIFrame(t *testing.T) {
	address := cemi.NewGroupAddr3(1, 2, 3)
	ldata := buildGroupOutbound(GroupEvent{Command: GroupWrite, Destination: address, Data: []byte{1}})
	ldata.Source = cemi.NewIndividualAddr3(1, 1, 10)
	message := &cemi.LDataInd{LData: ldata}
	frame := make([]byte, cemi.Size(message))
	cemi.Pack(frame, message)
	reports, err := knxusb.Encode(1, 3, frame)
	if err != nil {
		t.Fatal(err)
	}
	var assembler knxusb.Assembler
	var packet knxusb.Packet
	for _, report := range reports {
		packet, _, err = assembler.Add(report)
		if err != nil {
			t.Fatal(err)
		}
	}
	var decoded cemi.Message
	if _, err := safeUSBUnpack(packet.Body, &decoded); err != nil {
		t.Fatal(err)
	}
	ind, ok := decoded.(*cemi.LDataInd)
	if !ok || ind.Source != ldata.Source || ind.Destination != uint16(address) {
		t.Fatalf("wrong cEMI frame: %#v", decoded)
	}
	app, ok := ind.Data.(*cemi.AppData)
	if !ok || app.Command != cemi.GroupValueWrite || !bytes.Equal(app.Data, []byte{1}) {
		t.Fatalf("wrong group APDU: %#v", app)
	}
}
