package knx

import (
	"errors"
	"fmt"

	"github.com/vapourismo/knx-go/knx/cemi"
)

// cemiToEMI1 converts a cEMI L_Data frame into an EMI1 L_Data frame.
// EMI1 has no additional info and folds the address type and hop count into
// the length byte: bit 7 address type, bits 6-4 hop count, bits 3-0 NPDU length.
func cemiToEMI1(frame []byte) ([]byte, error) {
	if len(frame) < 2 {
		return nil, errors.New("short cEMI frame")
	}
	switch cemi.MessageCode(frame[0]) {
	case cemi.LDataReqCode, cemi.LDataIndCode, cemi.LDataConCode:
	default:
		return nil, fmt.Errorf("EMI1 cannot carry cEMI message %#x", frame[0])
	}
	frame = append([]byte(nil), frame...)
	frame[0] = cemiCodeToEMI1(frame[0])
	add := int(frame[1])
	if len(frame) < 2+add+7 {
		return nil, errors.New("short cEMI L_Data")
	}
	body := frame[2+add:]
	npduLen := int(body[6])
	if npduLen > 15 {
		return nil, errors.New("EMI1 does not support extended frames")
	}
	if len(body) < 7+npduLen+1 {
		return nil, errors.New("truncated cEMI TPDU")
	}
	ctrl := body[0]
	// EMI1 only has a 4-bit length. Short frames must be marked standard;
	// cEMI leaves that bit clear for escape APCI, which this interface rejects.
	if npduLen <= 15 {
		ctrl |= 0x80
	}
	out := make([]byte, 7+npduLen+1)
	out[0] = frame[0]
	out[1] = ctrl
	copy(out[2:4], body[2:4])
	copy(out[4:6], body[4:6])
	out[6] = (body[1] & 0xF0) | byte(npduLen)
	copy(out[7:], body[7:7+npduLen+1])
	return out, nil
}

// emi1ToCEMI converts an EMI1 L_Data frame into a cEMI L_Data frame with no
// additional info, so the rest of the stack can keep using cEMI.
func emi1ToCEMI(emi []byte) ([]byte, error) {
	if len(emi) < 1 {
		return nil, errors.New("short EMI1 frame")
	}
	code, ok := emi1CodeToCEMI(emi[0])
	if !ok {
		return nil, fmt.Errorf("unsupported EMI1 message %#x", emi[0])
	}
	emi = append([]byte(nil), emi...)
	emi[0] = code
	if len(emi) < 8 {
		return nil, errors.New("short EMI1 L_Data")
	}
	npduLen := int(emi[6] & 0x0F)
	if len(emi) < 7+npduLen+1 {
		return nil, errors.New("truncated EMI1 TPDU")
	}
	out := make([]byte, 9+npduLen+1)
	out[0] = code
	out[1] = 0
	out[2] = emi[1]
	out[3] = emi[6] & 0xF0
	copy(out[4:6], emi[2:4])
	copy(out[6:8], emi[4:6])
	out[8] = byte(npduLen)
	copy(out[9:], emi[7:7+npduLen+1])
	return out, nil
}

// EMI1 keeps the older link-layer message codes. cEMI renamed the
// indications and confirmations; requests stayed at 0x11.
func cemiCodeToEMI1(code byte) byte {
	switch cemi.MessageCode(code) {
	case cemi.LDataIndCode:
		return 0x49
	case cemi.LDataConCode:
		return 0x4E
	default:
		return code
	}
}

func emi1CodeToCEMI(code byte) (byte, bool) {
	switch code {
	case 0x11:
		return byte(cemi.LDataReqCode), true
	case 0x49:
		return byte(cemi.LDataIndCode), true
	case 0x4E:
		return byte(cemi.LDataConCode), true
	default:
		return 0, false
	}
}
