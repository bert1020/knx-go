package knx

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/vapourismo/knx-go/knx/cemi"
)

func TestUSBReadLocalProperty(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	u := &USB{file: writer, inbound: make(chan cemi.Message, 2), done: make(chan struct{})}
	u.inbound <- &cemi.UnsupportedMessage{Code: usbPropReadConfirm, Data: []byte{0, 0, 1, usbPIDMaxAPDULength, 0x10, 1, 0, 233}}
	value, err := u.ReadLocalProperty(context.Background(), 0, 1, usbPIDMaxAPDULength, 1, 1)
	if err != nil || !bytes.Equal(value, []byte{0, 233}) {
		t.Fatalf("value=%x err=%v", value, err)
	}
	report := make([]byte, usbReportSize)
	if _, err := io.ReadFull(reader, report); err != nil {
		t.Fatal(err)
	}
	packet, complete, err := new(usbAssembler).Add(report)
	if err != nil || !complete || !bytes.Equal(packet.body, []byte{0xfc, 0, 0, 1, usbPIDMaxAPDULength, 0x10, 1}) {
		t.Fatalf("request body=%x complete=%v err=%v", packet.body, complete, err)
	}
	u.inbound <- &cemi.UnsupportedMessage{Code: usbPropReadConfirm, Data: []byte{0, 0, 1, usbPIDMaxAPDULength, 0, 1, 7}}
	if _, err := u.ReadLocalProperty(context.Background(), 0, 1, usbPIDMaxAPDULength, 1, 1); err == nil {
		t.Fatal("negative property response accepted")
	}
}
