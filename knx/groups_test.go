package knx

import (
	"bytes"
	"testing"
	"time"

	"github.com/vapourismo/knx-go/knx/cemi"
)

func TestGroupEventFromLDataInd(t *testing.T) {
	dest := cemi.NewGroupAddr3(1, 2, 3)
	msg := &cemi.LDataInd{LData: cemi.LData{
		Control2:    cemi.Control2GroupAddr,
		Source:      cemi.NewIndividualAddr3(1, 1, 10),
		Destination: uint16(dest),
		Data:        &cemi.AppData{Command: cemi.GroupValueWrite, Data: []byte{0x41}},
	}}
	ev, ok := groupEventFrom(msg)
	if !ok {
		t.Fatal("group telegram not recognized")
	}
	if ev.Command != GroupWrite || ev.Source != msg.Source || ev.Destination != dest || !bytes.Equal(ev.Data, []byte{0x41}) {
		t.Fatalf("event = %+v", ev)
	}
	msg.Data.(*cemi.AppData).Data[0] = 0
	if ev.Data[0] != 0x41 {
		t.Fatal("group event shares the frame buffer")
	}

	msg.Control2 = 0
	if _, ok := groupEventFrom(msg); ok {
		t.Fatal("individual address accepted as a group telegram")
	}
}

func TestPublishGroupDropsWhenUnread(t *testing.T) {
	ch := make(chan GroupEvent, 1)
	done := make(chan struct{})
	msg := &cemi.LDataInd{LData: cemi.LData{
		Control2:    cemi.Control2GroupAddr,
		Destination: uint16(cemi.NewGroupAddr3(1, 2, 3)),
		Data:        &cemi.AppData{Command: cemi.GroupValueRead, Data: []byte{0}},
	}}
	publishGroup(ch, done, msg)
	publishGroup(ch, done, msg)
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("buffered group event missing")
	}
}
