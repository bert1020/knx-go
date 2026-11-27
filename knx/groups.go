// Copyright 2017 Ole Krüger.
// Licensed under the MIT license which can be found in the LICENSE file.

package knx

import (
	"github.com/vapourismo/knx-go/knx/cemi"
	"github.com/vapourismo/knx-go/knx/util"
)

// GroupCommand determines the meaning of a group event.
type GroupCommand uint8

// String generates a string representation of the command.
func (cmd GroupCommand) String() string {
	switch cmd {
	case GroupRead:
		return "Read"

	case GroupResponse:
		return "Response"

	case GroupWrite:
		return "Write"
	}

	return "Unknown"
}

// These are known group commands.
const (
	GroupRead     GroupCommand = 0
	GroupResponse GroupCommand = 1
	GroupWrite    GroupCommand = 2
)

// GroupEvent represents a group communication event.
type GroupEvent struct {
	Command     GroupCommand
	Source      cemi.IndividualAddr
	Destination cemi.GroupAddr
	Data        []byte
}

// A GroupClient is a KNX client which supports group communication.
type GroupClient interface {
	Send(event GroupEvent) error
	Inbound() <-chan GroupEvent
}

// groupEventFrom extracts a group telegram from a link-layer indication.
// KNXnet/IP tunnelling (03_08_04, data link layer) and a USB interface in
// cEMI/EMI1 link-layer mode (03_06_03) both deliver these as L_Data.ind:
// destination type group, APCI GroupValue_Read, GroupValue_Response or
// GroupValue_Write. Busmonitor (L_Busmon.ind) is a different, receive-only mode.
func groupEventFrom(msg cemi.Message) (GroupEvent, bool) {
	ind, ok := msg.(*cemi.LDataInd)
	if !ok || !ind.Control2.IsGroupAddr() {
		return GroupEvent{}, false
	}
	app, ok := ind.Data.(*cemi.AppData)
	if !ok || !app.Command.IsGroupCommand() {
		return GroupEvent{}, false
	}
	return GroupEvent{
		Command:     GroupCommand(app.Command),
		Source:      ind.Source,
		Destination: cemi.GroupAddr(ind.Destination),
		Data:        append([]byte(nil), app.Data...),
	}, true
}

// publishGroup forwards a group telegram without blocking the cEMI reader.
// A full channel drops the event so device traffic on the other channel keeps moving.
func publishGroup(ch chan GroupEvent, done <-chan struct{}, msg cemi.Message) {
	ev, ok := groupEventFrom(msg)
	if !ok || ch == nil {
		return
	}
	defer func() { recover() }()
	select {
	case ch <- ev:
	case <-done:
	default:
	}
}

// serveGroupInbound serves a group communication.
func serveGroupInbound(inbound <-chan cemi.Message, outbound chan<- GroupEvent) {
	util.Log(inbound, "Started worker")
	defer util.Log(inbound, "Worker exited")

	for msg := range inbound {
		if ev, ok := groupEventFrom(msg); ok {
			outbound <- ev
			continue
		}
		util.Log(inbound, "Received frame is not a group telegram")
	}

	close(outbound)
}

var defaultGroupLData = cemi.LData{
	Control1: cemi.Control1NoRepeat | cemi.Control1NoSysBroadcast | cemi.Control1WantAck | cemi.Control1Prio(cemi.PrioLow),
	Control2: cemi.Control2GroupAddr | cemi.Control2Hops(6),
}

// buildGroupOutbound constructs the L_Data core frame for group communication.
func buildGroupOutbound(event GroupEvent) cemi.LData {
	ldata := defaultGroupLData
	ldata.Data = &cemi.AppData{
		Command: cemi.APCI(event.Command),
		Data:    event.Data,
	}
	ldata.Source = event.Source
	ldata.Destination = uint16(event.Destination)

	if len(event.Data) <= 15 {
		ldata.Control1 |= cemi.Control1StdFrame
	}

	return ldata
}
