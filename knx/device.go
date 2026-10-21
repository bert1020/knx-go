package knx

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/vapourismo/knx-go/knx/cemi"
)

const (
	apciDeviceDescriptorRead        = 0x300
	apciDeviceDescriptorResponse    = 0x340
	apciMemoryRead                  = 0x200
	apciMemoryResponse              = 0x240
	apciMemoryWrite                 = 0x280
	apciPropertyValueRead           = 0x3d5
	apciPropertyValueResponse       = 0x3d6
	apciPropertyValueWrite          = 0x3d7
	apciPropertyDescriptionRead     = 0x3d8
	apciPropertyDescriptionResponse = 0x3d9
	apciFunctionPropertyCommand     = 0x3de
	apciFunctionPropertyStateRead   = 0x3df
	apciFunctionPropertyStateResp   = 0x3e0
	// Vendor APCI pair used by some vendor devices (e.g. RS485 gateways,
	// manufacturer 0x014B) for function-property style exchanges. Both ETS and
	// the vendor tooling were observed on the bus using 0x2C8/0x2C9; the
	// standard escape pair 0x3DF/0x3E0 is silently ignored by those devices.
	vendorFunctionPropertyStateRead = 0x2c8
	vendorFunctionPropertyStateResp = 0x2c9
	PIDObjectType                   = 1
	PIDMaskVersion                  = 11
	PIDDeviceManufacturer           = 12
	PIDIOList                       = 71
	PIDSerialNumber                 = 51
	PIDHardwareType                 = 78
)

// Device is a session addressed by a KNX individual (physical) address.
// Its methods serialize requests, so multiple goroutines may use one Device.
type Device struct {
	link            deviceConnection
	routing         bool
	source          cemi.IndividualAddr
	Address         cemi.IndividualAddr
	connected       bool
	inbound         chan *cemi.LDataInd
	done            chan struct{}
	linkDone        chan struct{}
	once            sync.Once
	onClose         func()
	requestMu       sync.Mutex
	sequence        uint8
	receiveSequence uint8
}

type deviceConnection interface {
	Send(cemi.Message) error
	Inbound() <-chan cemi.Message
}

// OpenDevice starts a connected device session over this existing tunnel.
// A Device consumes the tunnel's cEMI inbound stream until Device.Close.
func (t *Tunnel) OpenDevice(ctx context.Context, address cemi.IndividualAddr) (*Device, error) {
	return openDevice(ctx, t, 0, address, true)
}

// OpenDeviceConnectionless starts an unnumbered device session.
func (t *Tunnel) OpenDeviceConnectionless(address cemi.IndividualAddr) (*Device, error) {
	return openDevice(nil, t, 0, address, false)
}

// OpenDevice starts a connected session over an existing routing connection.
// source must be a unique physical address assigned to this client.
func (r *Router) OpenDevice(ctx context.Context, source, address cemi.IndividualAddr) (*Device, error) {
	return openDevice(ctx, r, source, address, true)
}

// OpenDeviceConnectionless starts an unnumbered routed device session.
func (r *Router) OpenDeviceConnectionless(source, address cemi.IndividualAddr) (*Device, error) {
	return openDevice(nil, r, source, address, false)
}

// OpenDevice starts a connected device session over this USB interface.
func (u *USB) OpenDevice(ctx context.Context, address cemi.IndividualAddr) (*Device, error) {
	return openDevice(ctx, u, 0, address, true)
}

// OpenDeviceConnectionless starts an unnumbered USB device session.
func (u *USB) OpenDeviceConnectionless(address cemi.IndividualAddr) (*Device, error) {
	return openDevice(nil, u, 0, address, false)
}

func openDevice(ctx context.Context, link deviceConnection, source, address cemi.IndividualAddr, connected bool) (*Device, error) {
	if link == nil {
		return nil, errors.New("nil cEMI connection")
	}
	if address == 0 {
		return nil, errors.New("physical address 0.0.0 is invalid")
	}
	_, routing := link.(*Router)
	if routing && source == 0 {
		return nil, errors.New("routing requires a source physical address")
	}
	d := &Device{link: link, routing: routing, source: source, Address: address, connected: connected, inbound: make(chan *cemi.LDataInd, 32), done: make(chan struct{}), linkDone: make(chan struct{})}
	go d.receive()
	if !connected {
		return d, nil
	}
	if ctx == nil {
		d.Close()
		return nil, errors.New("nil context")
	}
	if err := d.send(&cemi.ControlData{Command: 0}); err != nil {
		d.Close()
		return nil, err
	}
	// A device that already has a transport connection answers the first
	// T_Connect with T_Disconnect. Wait briefly, then connect once more.
	select {
	case frame := <-d.inbound:
		if frame != nil {
			if unit, ok := frame.Data.(*cemi.ControlData); ok && !unit.Numbered && unit.Command == 1 {
				if err := d.send(&cemi.ControlData{Command: 0}); err != nil {
					d.Close()
					return nil, err
				}
			}
		}
	case <-time.After(400 * time.Millisecond):
	case <-ctx.Done():
		d.Close()
		return nil, ctx.Err()
	}
	if _, err := d.ReadDeviceDescriptor(ctx); err != nil {
		d.Close()
		return nil, fmt.Errorf("connect to %s: %w", address, err)
	}
	return d, nil
}

func (d *Device) receive() {
	defer close(d.linkDone)
	for {
		select {
		case <-d.done:
			return
		case msg, ok := <-d.link.Inbound():
			if !ok {
				return
			}
			ind, ok := msg.(*cemi.LDataInd)
			if !ok || ind.Control2.IsGroupAddr() || ind.Source != d.Address {
				continue
			}
			select {
			case d.inbound <- ind:
			case <-d.done:
				return
			}
		}
	}
}

type PropertyDescription struct {
	ObjectIndex      uint8
	PropertyID       uint8
	PropertyIndex    uint8
	WriteEnabled     bool
	PropertyDataType uint8
	MaxElements      uint16
	ReadLevel        uint8
	WriteLevel       uint8
}

type InterfaceObject struct {
	Index uint8
	Type  uint16
}

func (d *Device) send(unit cemi.TransportUnit) error {
	select {
	case <-d.done:
		return errors.New("device is closed")
	case <-d.linkDone:
		return errors.New("cEMI connection is closed")
	default:
	}
	// The ack request bit must stay clear: devices do not answer management
	// frames that request a link-layer ack (verified against a SeaWin device
	// and by ETS, which never sets the bit either).
	control := cemi.Control1NoRepeat | cemi.Control1NoSysBroadcast | cemi.Control1Prio(cemi.PrioSystem)
	// Extended APCI (escape coding) must be sent in extended frame format:
	// the Extended Frame Format bit stays clear, or devices silently drop the frame.
	if app, ok := unit.(*cemi.AppData); ok && app.Command == cemi.Escape {
		// keep EFF (do not set Control1StdFrame)
	} else if unit.Size() <= 17 {
		control |= cemi.Control1StdFrame
	}
	frame := &cemi.LDataReq{LData: cemi.LData{
		Control1:    control,
		Control2:    cemi.Control2Hops(6),
		Destination: uint16(d.Address), Data: unit,
	}}
	if d.routing {
		ind := &cemi.LDataInd{LData: frame.LData}
		ind.Source = d.source
		return d.link.Send(ind)
	}
	return d.link.Send(frame)
}

func (d *Device) Close() {
	d.once.Do(func() {
		if d.connected {
			_ = d.send(&cemi.ControlData{Command: 1})
		}
		close(d.done)
		if d.onClose != nil {
			d.onClose()
		}
	})
}

func apci(app *cemi.AppData) uint16 {
	if len(app.Data) == 0 {
		return 0xffff
	}
	return uint16(app.Command)<<6 | uint16(app.Data[0]&0x3f)
}

func (d *Device) request(ctx context.Context, code, expected uint16, payload []byte) ([]byte, error) {
	d.requestMu.Lock()
	defer d.requestMu.Unlock()
	if ctx == nil {
		return nil, errors.New("nil context")
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 4*time.Second)
		defer cancel()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	app := &cemi.AppData{Numbered: d.connected, SeqNumber: d.sequence, Command: cemi.APCI(code >> 6), Data: append([]byte{byte(code & 0x3f)}, payload...)}
	// d.send relays through the tunnel's requestTunnel, whose resend loop runs
	// on its own ResponseTimeout (default 10s) and does not honor ctx. Run it in
	// a goroutine so the caller's deadline (e.g. a short management timeout)
	// also bounds the sending phase; an abandoned requestTunnel is bounded by
	// ResponseTimeout and released when the tunnel closes.
	sendErr := make(chan error, 1)
	go func() { sendErr <- d.send(app) }()
	select {
	case err := <-sendErr:
		if err != nil {
			return nil, err
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-d.done:
		return nil, errors.New("device is closed")
	case <-d.linkDone:
		return nil, errors.New("cEMI connection is closed")
	}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-d.done:
			return nil, errors.New("device is closed")
		case <-d.linkDone:
			return nil, errors.New("cEMI connection is closed")
		case frame := <-d.inbound:
			if frame == nil {
				continue
			}
			switch unit := frame.Data.(type) {
			case *cemi.ControlData:
				if !unit.Numbered && unit.Command == 1 {
					return nil, errors.New("device disconnected")
				}
				if d.connected && unit.Numbered && unit.SeqNumber == d.sequence && unit.Command == 3 {
					return nil, errors.New("device rejected transport packet")
				}
			case *cemi.AppData:
				// Escape-APCI responses carry the 10-bit APCI cleanly in
				// Command and Data[0]. Other responses share the first data
				// byte with APCI bits 5:0, so only the Command part must
				// match and Data[0] is response payload, not APCI.
				isEscape := unit.Command == cemi.Escape
				if isEscape {
					if apci(unit) != expected {
						continue
					}
				} else if apci(unit)&0x3c0 != expected&0x3c0 {
					continue
				}
				if d.connected {
					if !unit.Numbered || unit.SeqNumber != d.receiveSequence {
						continue
					}
					if err := d.send(&cemi.ControlData{Numbered: true, SeqNumber: unit.SeqNumber, Command: 2}); err != nil {
						return nil, err
					}
					d.receiveSequence = (d.receiveSequence + 1) & 15
					d.sequence = (d.sequence + 1) & 15
				}
				if isEscape {
					return append([]byte(nil), unit.Data[1:]...), nil
				}
				return append([]byte(nil), unit.Data...), nil
			}
		}
	}
}

// ReadDeviceDescriptor reads the two-byte KNX descriptor type 0.
func (d *Device) ReadDeviceDescriptor(ctx context.Context) (uint16, error) {
	data, err := d.request(ctx, apciDeviceDescriptorRead, apciDeviceDescriptorResponse, nil)
	if err != nil {
		return 0, err
	}
	// The response carries the descriptor type byte first, then the two
	// mask version bytes.
	if len(data) < 3 {
		return 0, errors.New("short device descriptor response")
	}
	return binary.BigEndian.Uint16(data[1:3]), nil
}

// ReadPropertyValue reads 1..15 elements of an interface object property.
// StartIndex is zero for the element count and one for the first value.
func (d *Device) ReadPropertyValue(ctx context.Context, objectIndex, propertyID, count uint8, startIndex uint16) ([]byte, error) {
	if count == 0 || count > 15 || startIndex > 0x0fff {
		return nil, errors.New("invalid property count or start index")
	}
	request := []byte{objectIndex, propertyID, count<<4 | byte(startIndex>>8), byte(startIndex)}
	data, err := d.request(ctx, apciPropertyValueRead, apciPropertyValueResponse, request)
	if err != nil {
		return nil, err
	}
	if len(data) < 4 || data[0] != objectIndex || data[1] != propertyID {
		return nil, errors.New("invalid property response")
	}
	gotCount := data[2] >> 4
	gotStart := uint16(data[2]&15)<<8 | uint16(data[3])
	if gotCount == 0 {
		return nil, fmt.Errorf("property %d on object %d rejected the read", propertyID, objectIndex)
	}
	if gotCount != count || gotStart != startIndex {
		return nil, errors.New("property response does not match request")
	}
	return append([]byte(nil), data[4:]...), nil
}

// WritePropertyValue writes 1..15 elements beginning at startIndex (1..4095).
// Value contains the encoded bytes for those elements, using the property's data type.
func (d *Device) WritePropertyValue(ctx context.Context, objectIndex, propertyID, count uint8, startIndex uint16, value []byte) error {
	if count == 0 || count > 15 || startIndex == 0 || startIndex > 0x0fff {
		return errors.New("invalid property count or start index")
	}
	if len(value) == 0 || len(value) > 250 {
		return errors.New("property value must contain 1..250 bytes")
	}
	request := make([]byte, 4, 4+len(value))
	request[0], request[1] = objectIndex, propertyID
	request[2], request[3] = count<<4|byte(startIndex>>8), byte(startIndex)
	request = append(request, value...)
	data, err := d.request(ctx, apciPropertyValueWrite, apciPropertyValueResponse, request)
	if err != nil {
		return err
	}
	if len(data) < 4 || data[0] != objectIndex || data[1] != propertyID {
		return errors.New("invalid property write response")
	}
	if data[2]>>4 == 0 {
		return fmt.Errorf("property %d on object %d rejected the write", propertyID, objectIndex)
	}
	if data[2]>>4 != count || uint16(data[2]&15)<<8|uint16(data[3]) != startIndex {
		return errors.New("property write response does not match request")
	}
	return nil
}

// ExchangePropertyValue writes 1..15 elements like WritePropertyValue but
// returns the raw element data of the property response instead of requiring
// an exact echo. Devices that run a request/response protocol inside a
// property answer with data that differs from the written value.
func (d *Device) ExchangePropertyValue(ctx context.Context, objectIndex, propertyID, count uint8, startIndex uint16, value []byte) ([]byte, error) {
	if count == 0 || count > 15 || startIndex == 0 || startIndex > 0x0fff {
		return nil, errors.New("invalid property count or start index")
	}
	if len(value) == 0 || len(value) > 250 {
		return nil, errors.New("property value must contain 1..250 bytes")
	}
	request := make([]byte, 4, 4+len(value))
	request[0], request[1] = objectIndex, propertyID
	request[2], request[3] = count<<4|byte(startIndex>>8), byte(startIndex)
	request = append(request, value...)
	data, err := d.request(ctx, apciPropertyValueWrite, apciPropertyValueResponse, request)
	if err != nil {
		return nil, err
	}
	if len(data) < 4 || data[0] != objectIndex || data[1] != propertyID {
		return nil, errors.New("invalid property write response")
	}
	if data[2]>>4 == 0 {
		return nil, fmt.Errorf("property %d on object %d rejected the write", propertyID, objectIndex)
	}
	return append([]byte(nil), data[4:]...), nil
}

// ReadFunctionPropertyState invokes A_FunctionProperty_StateRead on the given
// function property and returns the response's return code together with the
// response data. Both object index and property ID are echoed by the device;
// the request data follows them verbatim. A non-zero return code means the
// device processed the request but rejected it; the data may still be present.
func (d *Device) ReadFunctionPropertyState(ctx context.Context, objectIndex, propertyID uint8, data []byte) (uint8, []byte, error) {
	request := append([]byte{objectIndex, propertyID}, data...)
	response, err := d.request(ctx, apciFunctionPropertyStateRead, apciFunctionPropertyStateResp, request)
	if err != nil {
		return 0, nil, err
	}
	if len(response) < 3 || response[0] != objectIndex || response[1] != propertyID {
		return 0, nil, errors.New("invalid function property response")
	}
	return response[2], append([]byte(nil), response[3:]...), nil
}

// WriteFunctionPropertyCommand invokes A_FunctionProperty_Command on the given
// function property and returns the response's return code together with the
// response data. Some devices only implement the command variant of function
// property access; the response format matches A_FunctionProperty_StateRead.
func (d *Device) WriteFunctionPropertyCommand(ctx context.Context, objectIndex, propertyID uint8, data []byte) (uint8, []byte, error) {
	request := append([]byte{objectIndex, propertyID}, data...)
	response, err := d.request(ctx, apciFunctionPropertyCommand, apciFunctionPropertyStateResp, request)
	if err != nil {
		return 0, nil, err
	}
	if len(response) < 3 || response[0] != objectIndex || response[1] != propertyID {
		return 0, nil, errors.New("invalid function property response")
	}
	return response[2], append([]byte(nil), response[3:]...), nil
}

// ReadVendorPropertyState exchanges a function-property style state request
// using the vendor-specific APCI pair 0x2C8/0x2C9. The request and response
// payloads match A_FunctionProperty_StateRead: [objectIndex][propertyID] plus
// the request data, answered by [objectIndex][propertyID][returnCode][data].
// Unlike the escape-coded pair, the response's first data byte holds the low
// APCI bits (0x09 for 0x2C9) instead of payload, so it is stripped here and
// the echoed object/property follow at offsets 1 and 2.
func (d *Device) ReadVendorPropertyState(ctx context.Context, objectIndex, propertyID uint8, data []byte) (uint8, []byte, error) {
	request := append([]byte{objectIndex, propertyID}, data...)
	response, err := d.request(ctx, vendorFunctionPropertyStateRead, vendorFunctionPropertyStateResp, request)
	if err != nil {
		return 0, nil, err
	}
	if len(response) < 4 || response[1] != objectIndex || response[2] != propertyID {
		return 0, nil, errors.New("invalid vendor property response")
	}
	return response[3], append([]byte(nil), response[4:]...), nil
}

// WriteDeviceManufacturer writes the manufacturer code of the device object
// (object 0, property 12). It fails and leaves the device unchanged when the
// property is not write enabled or the device rejects the write.
func (d *Device) WriteDeviceManufacturer(ctx context.Context, code uint16) error {
	desc, err := d.ReadPropertyDescriptionByID(ctx, 0, PIDDeviceManufacturer)
	if err != nil {
		return fmt.Errorf("manufacturer code property description: %w", err)
	}
	if !desc.WriteEnabled {
		return errors.New("manufacturer code property is write protected")
	}
	return d.WritePropertyValue(ctx, 0, PIDDeviceManufacturer, 1, 1, []byte{byte(code >> 8), byte(code)})
}

func (d *Device) ReadPropertyElementCount(ctx context.Context, objectIndex, propertyID uint8) (uint16, error) {
	data, err := d.ReadPropertyValue(ctx, objectIndex, propertyID, 1, 0)
	if err != nil {
		return 0, err
	}
	if len(data) != 2 {
		return 0, fmt.Errorf("property element count has %d bytes", len(data))
	}
	return binary.BigEndian.Uint16(data), nil
}

func (d *Device) readPropertyDescription(ctx context.Context, objectIndex, propertyID, propertyIndex uint8) (PropertyDescription, error) {
	data, err := d.request(ctx, apciPropertyDescriptionRead, apciPropertyDescriptionResponse, []byte{objectIndex, propertyID, propertyIndex})
	if err != nil {
		return PropertyDescription{}, err
	}
	if len(data) < 7 || data[0] != objectIndex {
		return PropertyDescription{}, errors.New("invalid property description response")
	}
	if propertyID != 0 && data[1] != propertyID {
		return PropertyDescription{}, errors.New("property ID mismatch")
	}
	if propertyID == 0 && data[2] != propertyIndex {
		return PropertyDescription{}, errors.New("property index mismatch")
	}
	return PropertyDescription{
		ObjectIndex: data[0], PropertyID: data[1], PropertyIndex: data[2],
		WriteEnabled: data[3]&0x80 != 0, PropertyDataType: data[3] & 0x3f,
		MaxElements: binary.BigEndian.Uint16(data[4:6]) & 0x0fff,
		ReadLevel:   data[6] >> 4, WriteLevel: data[6] & 15,
	}, nil
}

func (d *Device) ReadPropertyDescriptionByID(ctx context.Context, objectIndex, propertyID uint8) (PropertyDescription, error) {
	if propertyID == 0 {
		return PropertyDescription{}, errors.New("property ID 0 requires ReadPropertyDescriptionByIndex")
	}
	return d.readPropertyDescription(ctx, objectIndex, propertyID, 0)
}

func (d *Device) ReadPropertyDescriptionByIndex(ctx context.Context, objectIndex, propertyIndex uint8) (PropertyDescription, error) {
	return d.readPropertyDescription(ctx, objectIndex, 0, propertyIndex)
}

func (d *Device) ReadInterfaceObjectType(ctx context.Context, objectIndex uint8) (uint16, error) {
	data, err := d.ReadPropertyValue(ctx, objectIndex, PIDObjectType, 1, 1)
	if err != nil {
		return 0, err
	}
	if len(data) != 2 {
		return 0, fmt.Errorf("object type has %d bytes", len(data))
	}
	return binary.BigEndian.Uint16(data), nil
}

// ReadInterfaceObjects reads PID_IO_LIST from device object 0 and returns
// each implemented interface object type with its object index.
func (d *Device) ReadInterfaceObjects(ctx context.Context) ([]InterfaceObject, error) {
	count, err := d.ReadPropertyElementCount(ctx, 0, PIDIOList)
	if err != nil {
		return nil, err
	}
	if count > 255 {
		return nil, fmt.Errorf("device reports %d interface objects; maximum supported is 255", count)
	}
	objects := make([]InterfaceObject, 0, count)
	for i := uint16(1); i <= count; i++ {
		data, err := d.ReadPropertyValue(ctx, 0, PIDIOList, 1, i)
		if err != nil {
			return objects, fmt.Errorf("interface object %d: %w", i, err)
		}
		if len(data) != 2 {
			return objects, fmt.Errorf("interface object %d has %d bytes", i, len(data))
		}
		objects = append(objects, InterfaceObject{Index: uint8(i - 1), Type: binary.BigEndian.Uint16(data)})
	}
	return objects, nil
}

// ReadMemory reads 1..63 octets of memory starting at address. The A_Memory
// protocol limits the octet counter to six bits. Requests larger than a
// standard frame use extended framing automatically.
func (d *Device) ReadMemory(ctx context.Context, address uint16, count uint8) ([]byte, error) {
	if count == 0 || count > 63 {
		return nil, errors.New("memory read count must be 1..63")
	}
	data, err := d.request(ctx, apciMemoryRead, apciMemoryResponse, []byte{count, byte(address >> 8), byte(address)})
	if err != nil {
		return nil, err
	}
	if len(data) < 3 {
		return nil, errors.New("short memory read response")
	}
	if data[0] != count || uint16(data[1])<<8|uint16(data[2]) != address {
		return nil, errors.New("memory read response does not match request")
	}
	if len(data) < 3+int(count) {
		return nil, errors.New("short memory read payload")
	}
	return append([]byte(nil), data[3:3+count]...), nil
}

// WriteMemory writes 1..63 octets of data to memory starting at address. The
// device echoes the written octets; the write is only acknowledged when the
// echo matches the request.
func (d *Device) WriteMemory(ctx context.Context, address uint16, data []byte) error {
	if len(data) == 0 || len(data) > 63 {
		return errors.New("memory write data must contain 1..63 octets")
	}
	payload := append([]byte{byte(len(data)), byte(address >> 8), byte(address)}, data...)
	resp, err := d.request(ctx, apciMemoryWrite, apciMemoryResponse, payload)
	if err != nil {
		return err
	}
	if len(resp) < 3 {
		return errors.New("short memory write response")
	}
	if resp[0] != byte(len(data)) || uint16(resp[1])<<8|uint16(resp[2]) != address {
		return errors.New("memory write response does not match request")
	}
	if !bytes.Equal(resp[3:], data) {
		return errors.New("memory write response does not echo written data")
	}
	return nil
}
