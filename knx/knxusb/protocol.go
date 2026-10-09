// Package knxusb implements the KNX USB HID transport: report assembly and
// encoding, the operating system HID ports and interface enumeration. The
// cEMI session layer lives in the parent knx package.
package knxusb

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// ReportSize is the fixed size of a KNX USB HID report.
const ReportSize = 64

// A Packet is one complete KNX USB transfer. Protocol 0x0f carries control
// services, protocol 0x01 carries cEMI frames.
type Packet struct {
	Protocol byte
	Service  byte
	Body     []byte
}

// An Assembler reassembles KNX USB transfers from fixed-size HID reports.
// It is not safe for concurrent use.
type Assembler struct {
	next byte
	data []byte
}

// Add feeds one HID report into the assembler. It returns the completed
// transfer when the end flag is set. A report with unexpected sequencing
// resets the assembler, so the next transfer starts over cleanly.
func (a *Assembler) Add(report []byte) (Packet, bool, error) {
	if len(report) < 3 || report[0] != 1 {
		return Packet{}, false, errors.New("invalid KNX USB HID report")
	}
	length := int(report[2])
	if length > len(report)-3 {
		return Packet{}, false, errors.New("truncated KNX USB HID report")
	}
	seq, flags := report[1]>>4, report[1]&7
	if flags&1 != 0 {
		if seq != 1 {
			return Packet{}, false, errors.New("invalid KNX USB start sequence")
		}
		a.data = append(a.data[:0], report[3:3+length]...)
		a.next = 2
	} else {
		if a.next == 0 || seq != a.next || flags&4 == 0 {
			a.next = 0
			return Packet{}, false, errors.New("invalid KNX USB continuation")
		}
		a.next++
		a.data = append(a.data, report[3:3+length]...)
	}
	if len(a.data) > 512 {
		a.next = 0
		return Packet{}, false, errors.New("KNX USB transfer exceeds 512 bytes")
	}
	if flags&2 == 0 {
		return Packet{}, false, nil
	}
	a.next = 0
	if len(a.data) < 8 || a.data[0] != 0 || a.data[1] != 8 {
		return Packet{}, false, errors.New("invalid KNX USB transfer header")
	}
	length = int(binary.BigEndian.Uint16(a.data[2:4]))
	if len(a.data) != length+8 {
		return Packet{}, false, fmt.Errorf("KNX USB transfer length %d, actual %d", length, len(a.data)-8)
	}
	return Packet{Protocol: a.data[4], Service: a.data[5], Body: append([]byte(nil), a.data[8:]...)}, true, nil
}

// Encode splits a KNX USB transfer into fixed-size HID reports.
func Encode(protocol, service byte, body []byte) ([][]byte, error) {
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
		r := make([]byte, ReportSize)
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
