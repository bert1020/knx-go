package knx

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const usbReportSize = 64

type usbPacket struct {
	protocol byte
	service  byte
	body     []byte
}

type usbAssembler struct {
	next byte
	data []byte
}

func (a *usbAssembler) Add(report []byte) (usbPacket, bool, error) {
	if len(report) < 3 || report[0] != 1 {
		return usbPacket{}, false, errors.New("invalid KNX USB HID report")
	}
	length := int(report[2])
	if length > len(report)-3 {
		return usbPacket{}, false, errors.New("truncated KNX USB HID report")
	}
	seq, flags := report[1]>>4, report[1]&7
	if flags&1 != 0 {
		if seq != 1 {
			return usbPacket{}, false, errors.New("invalid KNX USB start sequence")
		}
		a.data = append(a.data[:0], report[3:3+length]...)
		a.next = 2
	} else {
		if a.next == 0 || seq != a.next || flags&4 == 0 {
			a.next = 0
			return usbPacket{}, false, errors.New("invalid KNX USB continuation")
		}
		a.next++
		a.data = append(a.data, report[3:3+length]...)
	}
	if len(a.data) > 512 {
		a.next = 0
		return usbPacket{}, false, errors.New("KNX USB transfer exceeds 512 bytes")
	}
	if flags&2 == 0 {
		return usbPacket{}, false, nil
	}
	a.next = 0
	if len(a.data) < 8 || a.data[0] != 0 || a.data[1] != 8 {
		return usbPacket{}, false, errors.New("invalid KNX USB transfer header")
	}
	length = int(binary.BigEndian.Uint16(a.data[2:4]))
	if len(a.data) != length+8 {
		return usbPacket{}, false, fmt.Errorf("KNX USB transfer length %d, actual %d", length, len(a.data)-8)
	}
	return usbPacket{protocol: a.data[4], service: a.data[5], body: append([]byte(nil), a.data[8:]...)}, true, nil
}

func usbEncode(protocol, service byte, body []byte) ([][]byte, error) {
	if len(body) > 263 {
		return nil, errors.New("KNX USB payload exceeds 263 bytes")
	}
	transfer := make([]byte, 8+len(body))
	transfer[1] = 8
	binary.BigEndian.PutUint16(transfer[2:4], uint16(len(body)))
	transfer[4], transfer[5] = protocol, service
	copy(transfer[8:], body)
	var reports [][]byte
	for len(transfer) != 0 {
		if len(reports) >= 15 {
			return nil, errors.New("too many KNX USB fragments")
		}
		part := len(transfer)
		if part > 61 {
			part = 61
		}
		r := make([]byte, usbReportSize)
		r[0] = 1
		r[1] = byte(len(reports)+1) << 4
		if len(reports) == 0 {
			r[1] |= 1
		} else {
			r[1] |= 4
		}
		if len(transfer) == part {
			r[1] |= 2
		} else {
			r[1] |= 4
		}
		r[2] = byte(part)
		copy(r[3:], transfer[:part])
		reports = append(reports, r)
		transfer = transfer[part:]
	}
	return reports, nil
}
