package knxusb

import (
	"bytes"
	"testing"
)

func TestUSBTransferRoundTrip(t *testing.T) {
	for _, size := range []int{0, 1, 53, 54, 100, 263} {
		body := bytes.Repeat([]byte{0x29}, size)
		reports, err := Encode(1, 3, body)
		if err != nil {
			t.Fatal(err)
		}
		var decoder Assembler
		for i, report := range reports {
			got, done, err := decoder.Add(report)
			if err != nil {
				t.Fatal(err)
			}
			if done != (i == len(reports)-1) {
				t.Fatalf("size %d fragment %d: done=%v", size, i, done)
			}
			if done && (got.Protocol != 1 || got.Service != 3 || !bytes.Equal(got.Body, body)) {
				t.Fatalf("size %d: wrong transfer", size)
			}
		}
	}
}

func TestUSBRejectsBadContinuation(t *testing.T) {
	reports, err := Encode(1, 3, bytes.Repeat([]byte{1}, 100))
	if err != nil {
		t.Fatal(err)
	}
	var decoder Assembler
	_, _, _ = decoder.Add(reports[0])
	reports[1][1] = 0x34
	if _, _, err := decoder.Add(reports[1]); err == nil {
		t.Fatal("accepted wrong sequence")
	}
}
