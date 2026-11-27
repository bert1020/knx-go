package knx

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/vapourismo/knx-go/knx/cemi"
)

type fakeLink struct {
	inbound chan cemi.Message
	sent    []cemi.Message
}

func newFakeLink() *fakeLink {
	return &fakeLink{inbound: make(chan cemi.Message, 32)}
}

func (f *fakeLink) Send(msg cemi.Message) error {
	f.sent = append(f.sent, msg)
	req, ok := msg.(*cemi.LDataReq)
	if !ok {
		return nil
	}
	con := &cemi.LDataCon{LData: req.LData}
	con.Source = 0xFFFE
	f.push(con)
	return nil
}

func (f *fakeLink) Inbound() <-chan cemi.Message { return f.inbound }

func (f *fakeLink) push(msg cemi.Message) {
	select {
	case f.inbound <- msg:
	case <-time.After(2 * time.Second):
		panic("fake inbound blocked")
	}
}

func TestScanAddressesFindsDeviceByDisconnect(t *testing.T) {
	link := newFakeLink()
	target := cemi.NewIndividualAddr3(1, 1, 2)
	serial := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}
	var armed bool

	origSend := link.Send
	linkSend := func(msg cemi.Message) error {
		req, _ := msg.(*cemi.LDataReq)
		err := origSend(msg)
		if req == nil {
			return err
		}
		if c, ok := req.Data.(*cemi.ControlData); ok && !c.Numbered && c.Command == 0 &&
			cemi.IndividualAddr(req.Destination) == target && !armed {
			armed = true
			time.AfterFunc(30*time.Millisecond, func() {
				link.push(&cemi.LDataInd{LData: cemi.LData{
					Source:      target,
					Destination: 0xFFFE,
					Control2:    cemi.Control2Hops(6),
					Data:        &cemi.ControlData{Command: 1},
				}})
			})
		}
		if app, ok := req.Data.(*cemi.AppData); ok && cemi.IndividualAddr(req.Destination) == target {
			switch app.Command {
			case cemi.MaskVersionRead:
				link.push(&cemi.LDataInd{LData: cemi.LData{
					Source:      target,
					Destination: 0xFFFE,
					Control2:    cemi.Control2Hops(6),
					Data: &cemi.AppData{
						Numbered:  true,
						SeqNumber: app.SeqNumber,
						Command:   cemi.MaskVersionResponse,
						Data:      []byte{0x00, 0x07, 0xB0},
					},
				}})
			case cemi.Escape:
				link.push(&cemi.LDataInd{LData: cemi.LData{
					Source:      target,
					Destination: 0xFFFE,
					Control2:    cemi.Control2Hops(6),
					Data: &cemi.AppData{
						Numbered:  true,
						SeqNumber: app.SeqNumber,
						Command:   cemi.Escape,
						Data:      append([]byte{apciPropertyValueResponse & 0x3f, 0, pidDeviceSerial, 0x10, 1}, serial...),
					},
				}})
			}
		}
		return err
	}

	// Wrap Send so the scanner and the scripted device share one implementation.
	scripted := &scriptedLink{fakeLink: link, send: linkSend}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	got, err := ScanAddresses(ctx, scripted, []cemi.IndividualAddr{
		cemi.NewIndividualAddr3(1, 1, 1),
		target,
		cemi.NewIndividualAddr3(1, 1, 3),
	}, AddressScanConfig{
		Interval:       5 * time.Millisecond,
		DisconnectWait: 100 * time.Millisecond,
		ReadTimeout:    500 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("devices = %+v, want only 1.1.2", got)
	}
	if got[0].Address != target || !got[0].HasMask || got[0].MaskVersion != 0x07B0 {
		t.Fatalf("mask = %+v", got[0])
	}
	if !bytes.Equal(got[0].Serial, serial) {
		t.Fatalf("serial = %x", got[0].Serial)
	}

	probes := map[cemi.IndividualAddr]int{}
	for _, msg := range link.sent {
		req, ok := msg.(*cemi.LDataReq)
		if !ok {
			continue
		}
		c, ok := req.Data.(*cemi.ControlData)
		if !ok || c.Numbered || c.Command != 0 {
			continue
		}
		probes[cemi.IndividualAddr(req.Destination)]++
	}
	if probes[cemi.NewIndividualAddr3(1, 1, 1)] != 1 || probes[target] != 2 || probes[cemi.NewIndividualAddr3(1, 1, 3)] != 1 {
		t.Fatalf("T_Connect counts = %v", probes)
	}
}

func TestScanAddressesRetriesIncompleteRead(t *testing.T) {
	link := newFakeLink()
	target := cemi.NewIndividualAddr3(1, 1, 2)
	serial := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}
	var armed bool
	maskReads := 0

	scripted := &scriptedLink{fakeLink: link, send: func(msg cemi.Message) error {
		req, _ := msg.(*cemi.LDataReq)
		err := link.Send(msg)
		if req == nil {
			return err
		}
		if c, ok := req.Data.(*cemi.ControlData); ok && !c.Numbered && c.Command == 0 &&
			cemi.IndividualAddr(req.Destination) == target && !armed {
			armed = true
			time.AfterFunc(20*time.Millisecond, func() {
				link.push(&cemi.LDataInd{LData: cemi.LData{
					Source:      target,
					Destination: 0xFFFE,
					Control2:    cemi.Control2Hops(6),
					Data:        &cemi.ControlData{Command: 1},
				}})
			})
		}
		app, ok := req.Data.(*cemi.AppData)
		if !ok || cemi.IndividualAddr(req.Destination) != target {
			return err
		}
		switch app.Command {
		case cemi.MaskVersionRead:
			maskReads++
			if maskReads == 1 {
				return err
			}
			link.push(&cemi.LDataInd{LData: cemi.LData{
				Source:      target,
				Destination: 0xFFFE,
				Control2:    cemi.Control2Hops(6),
				Data: &cemi.AppData{
					Numbered:  true,
					SeqNumber: app.SeqNumber,
					Command:   cemi.MaskVersionResponse,
					Data:      []byte{0x00, 0x07, 0xB0},
				},
			}})
		case cemi.Escape:
			link.push(&cemi.LDataInd{LData: cemi.LData{
				Source:      target,
				Destination: 0xFFFE,
				Control2:    cemi.Control2Hops(6),
				Data: &cemi.AppData{
					Numbered:  true,
					SeqNumber: app.SeqNumber,
					Command:   cemi.Escape,
					Data:      append([]byte{apciPropertyValueResponse & 0x3f, 0, pidDeviceSerial, 0x10, 1}, serial...),
				},
			}})
		}
		return err
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	got, err := ScanAddresses(ctx, scripted, []cemi.IndividualAddr{target}, AddressScanConfig{
		Interval:       5 * time.Millisecond,
		DisconnectWait: 80 * time.Millisecond,
		ReadTimeout:    150 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 1 || !got[0].HasMask || got[0].MaskVersion != 0x07B0 || !bytes.Equal(got[0].Serial, serial) {
		t.Fatalf("device = %+v", got)
	}
	if maskReads != 2 {
		t.Fatalf("mask reads = %d, want a dropped read then one success", maskReads)
	}
}

func TestScanAddressesHonorsSingleAttempt(t *testing.T) {
	link := newFakeLink()
	target := cemi.NewIndividualAddr3(1, 1, 2)
	var armed bool
	maskReads := 0
	scripted := &scriptedLink{fakeLink: link, send: func(msg cemi.Message) error {
		req, _ := msg.(*cemi.LDataReq)
		err := link.Send(msg)
		if req == nil {
			return err
		}
		if c, ok := req.Data.(*cemi.ControlData); ok && !c.Numbered && c.Command == 0 &&
			cemi.IndividualAddr(req.Destination) == target && !armed {
			armed = true
			time.AfterFunc(20*time.Millisecond, func() {
				link.push(&cemi.LDataInd{LData: cemi.LData{
					Source:      target,
					Destination: 0xFFFE,
					Control2:    cemi.Control2Hops(6),
					Data:        &cemi.ControlData{Command: 1},
				}})
			})
		}
		if app, ok := req.Data.(*cemi.AppData); ok && app.Command == cemi.MaskVersionRead {
			maskReads++
		}
		return err
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got, err := ScanAddresses(ctx, scripted, []cemi.IndividualAddr{target}, AddressScanConfig{
		Interval:       5 * time.Millisecond,
		DisconnectWait: 80 * time.Millisecond,
		ReadTimeout:    80 * time.Millisecond,
		ReadAttempts:   1,
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 1 || got[0].HasMask || maskReads != 1 {
		t.Fatalf("devices=%+v maskReads=%d", got, maskReads)
	}
}

func TestScanAddressesCancel(t *testing.T) {
	link := newFakeLink()
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(15*time.Millisecond, cancel)
	start := time.Now()
	_, err := ScanLine(ctx, link, 1, 1, AddressScanConfig{
		Interval:       20 * time.Millisecond,
		DisconnectWait: 5 * time.Second,
		ReadTimeout:    time.Second,
	})
	if time.Since(start) > time.Second {
		t.Fatalf("cancel took %s", time.Since(start))
	}
	if err != context.Canceled {
		t.Fatalf("err = %v", err)
	}
}

func TestReadDevicesSendsShortPropertyReadAsStandardFrame(t *testing.T) {
	link := newFakeLink()
	first := cemi.NewIndividualAddr3(1, 1, 2)
	second := cemi.NewIndividualAddr3(1, 1, 3)
	serial := []byte{0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F}
	var extended, standard int
	scripted := &scriptedLink{fakeLink: link, send: func(msg cemi.Message) error {
		req, _ := msg.(*cemi.LDataReq)
		err := link.Send(msg)
		if req == nil {
			return err
		}
		app, ok := req.Data.(*cemi.AppData)
		if !ok {
			return err
		}
		switch app.Command {
		case cemi.MaskVersionRead:
			link.push(&cemi.LDataInd{LData: cemi.LData{
				Source:      cemi.IndividualAddr(req.Destination),
				Destination: 0xFFFE,
				Control2:    cemi.Control2Hops(6),
				Data: &cemi.AppData{
					Numbered:  true,
					SeqNumber: app.SeqNumber,
					Command:   cemi.MaskVersionResponse,
					Data:      []byte{0x00, 0x07, 0xB0},
				},
			}})
		case cemi.Escape:
			if req.Control1&cemi.Control1StdFrame == 0 {
				extended++
				return err
			}
			standard++
			link.push(&cemi.LDataInd{LData: cemi.LData{
				Source:      cemi.IndividualAddr(req.Destination),
				Destination: 0xFFFE,
				Control2:    cemi.Control2Hops(6),
				Data: &cemi.AppData{
					Numbered:  true,
					SeqNumber: app.SeqNumber,
					Command:   cemi.Escape,
					Data:      append([]byte{apciPropertyValueResponse & 0x3f, 0, pidDeviceSerial, 0x10, 1}, serial...),
				},
			}})
		}
		return err
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got, err := ReadDevices(ctx, scripted, []cemi.IndividualAddr{first, second}, AddressScanConfig{
		ReadTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 2 || !bytes.Equal(got[0].Serial, serial) || !bytes.Equal(got[1].Serial, serial) {
		t.Fatalf("devices = %+v", got)
	}
	if extended != 0 || standard != 2 {
		t.Fatalf("extended frames = %d, standard frames = %d, want a standard frame for each device", extended, standard)
	}
}

func TestReadDevicesReadsWithoutProbeWait(t *testing.T) {
	link := newFakeLink()
	target := cemi.NewIndividualAddr3(1, 1, 2)
	serial := []byte{0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F}
	connects := 0
	scripted := &scriptedLink{fakeLink: link, send: func(msg cemi.Message) error {
		req, _ := msg.(*cemi.LDataReq)
		err := link.Send(msg)
		if req == nil {
			return err
		}
		if c, ok := req.Data.(*cemi.ControlData); ok && !c.Numbered && c.Command == 0 {
			connects++
		}
		app, ok := req.Data.(*cemi.AppData)
		if !ok || cemi.IndividualAddr(req.Destination) != target {
			return err
		}
		switch app.Command {
		case cemi.MaskVersionRead:
			link.push(&cemi.LDataInd{LData: cemi.LData{
				Source:      target,
				Destination: 0xFFFE,
				Control2:    cemi.Control2Hops(6),
				Data: &cemi.AppData{
					Numbered:  true,
					SeqNumber: app.SeqNumber,
					Command:   cemi.MaskVersionResponse,
					Data:      []byte{0x00, 0x07, 0xB0},
				},
			}})
		case cemi.Escape:
			link.push(&cemi.LDataInd{LData: cemi.LData{
				Source:      target,
				Destination: 0xFFFE,
				Control2:    cemi.Control2Hops(6),
				Data: &cemi.AppData{
					Numbered:  true,
					SeqNumber: app.SeqNumber,
					Command:   cemi.Escape,
					Data:      append([]byte{apciPropertyValueResponse & 0x3f, 0, pidDeviceSerial, 0x10, 1}, serial...),
				},
			}})
		}
		return err
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	got, err := ReadDevices(ctx, scripted, []cemi.IndividualAddr{target}, AddressScanConfig{
		DisconnectWait: 30 * time.Second,
		ReadTimeout:    200 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("read waited %s", time.Since(start))
	}
	if len(got) != 1 || !got[0].HasMask || got[0].MaskVersion != 0x07B0 || !bytes.Equal(got[0].Serial, serial) {
		t.Fatalf("device = %+v", got)
	}
	if connects != 1 {
		t.Fatalf("T_Connect = %d, want one management connect", connects)
	}
}

type scriptedLink struct {
	*fakeLink
	send func(cemi.Message) error
}

func (s *scriptedLink) Send(msg cemi.Message) error { return s.send(msg) }
