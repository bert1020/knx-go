package knx

// 物理地址扫描，过程与 ETS「行扫描」一致（依据 USB 抓包）：
//
//  1. 按约 60ms 的间隔向每个物理地址只发 T_Connect，不等待设备应答，
//     也不靠 L_Data.con 判断在线。cEMI 对空地址也回成功确认；EMI1 的负确认
//     只表示本机没收到链路应答，不能据此丢掉这个地址。
//  2. 在线设备会建立传输层连接；连接空闲约 6 秒后设备主动发 T_Disconnect。
//     抓包里 1.1.2 在 T_Connect 之后 6.1 秒回了 T_Disconnect。
//  3. 收到 T_Disconnect 才打开一次管理连接，读设备描述符（掩码版本）。
//     抓包里的应答是 APCI 0x340、数据 00 07 B0，即掩码 07B0。这次交换没有序列号。
//  4. 序列号不在扫描帧里，连接还开着时再读设备对象属性 11（PID_SERIAL_NUMBER）。
//     属性 11 被拒绝时再试属性 51。
//  5. 总线忙时这次管理连接可能丢帧或被拆掉。掩码或序列号没读到就拆链，
//     间隔 300ms 再读，最多 3 次。探测间隔不变。设备明确拒绝序列号属性时不重试。
//
// 扫描期间该链路上不能再有其他读者，Inbound 由扫描独占。

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/vapourismo/knx-go/knx/cemi"
)

const (
	// pidDeviceSerial 是设备对象的序列号（03_05_01 PID_SERIAL_NUMBER）。
	pidDeviceSerial = PIDSerialNumber
	// pidSerialNumberAlt 不是标准序列号。个别设备把序列号放在属性 51，标准里 51 是路由器的 PID_ROUTING_COUNT。
	pidSerialNumberAlt = 51
	// readAttempts 是同一台已发现设备的管理读取次数，含第一次。
	readAttempts = 3
	// readRetryGap 让设备处理完上一次 T_Disconnect，并躲开总线上的一次冲突。
	readRetryGap = 300 * time.Millisecond
)

// Link 是支持原始 cEMI 收发的链路。*Tunnel 与 *USB 均满足该接口。
type Link interface {
	Send(msg cemi.Message) error
	Inbound() <-chan cemi.Message
}

// LineDevice 是一次扫描读到的设备。HasMask 为 false 时 MaskVersion 无意义。
// Serial 为空表示描述符读到了、但序列号没有读到。
type LineDevice struct {
	Address     cemi.IndividualAddr
	MaskVersion uint16
	HasMask     bool
	Serial      []byte
}

// AddressScanEvent 是扫描进度。Phase 为 probe、wait 或 device。
type AddressScanEvent struct {
	Phase   string
	Address cemi.IndividualAddr
	Index   int
	Total   int
	Found   int
	Device  *LineDevice
}

// AddressScanConfig 扫描参数，零值即默认配置。
type AddressScanConfig struct {
	// 两个 T_Connect 之间的间隔，零值 60ms，与 ETS 抓包一致。
	Interval time.Duration
	// 最后一个 T_Connect 之后继续收集 T_Disconnect 的时间，零值 7s。
	// 设备连接超时约 6s，抓包实测 6.1s。
	DisconnectWait time.Duration
	// 单次描述符或属性读的应答上限，零值 2s。
	ReadTimeout time.Duration
	// 管理连接发出后、读掩码之前的等待，零值 30ms。
	ConnectSettle time.Duration
	// 同一次设备读取没读全时，下一次尝试前的间隔，零值 300ms。
	RetryGap time.Duration
	// 同一台已发现设备的读取次数，含第一次。零值 3。
	ReadAttempts int
	// Progress 返回 false 时停止继续探测，已发现的设备仍会读完。
	Progress func(AddressScanEvent) bool
}

// LineAddresses 生成一条支线上 [first, last] 的物理地址（含两端，设备号 0..255）。
func LineAddresses(area, line, first, last uint8) []cemi.IndividualAddr {
	if last < first {
		first, last = last, first
	}
	out := make([]cemi.IndividualAddr, 0, int(last-first)+1)
	for dev := int(first); dev <= int(last); dev++ {
		out = append(out, cemi.NewIndividualAddr3(area, line, uint8(dev)))
	}
	return out
}

// ScanLine 扫描一条支线的全部设备号 0..255。
func ScanLine(ctx context.Context, link Link, area, line uint8, cfg AddressScanConfig) ([]LineDevice, error) {
	return ScanAddresses(ctx, link, LineAddresses(area, line, 0, 255), cfg)
}

// ScanAddresses 按给定顺序探测物理地址，返回在线设备的掩码版本和序列号。
// 取消 ctx 会立刻停止，已读完的设备仍然返回，同时返回 ctx.Err()。
func ScanAddresses(ctx context.Context, link Link, addrs []cemi.IndividualAddr, cfg AddressScanConfig) ([]LineDevice, error) {
	if link == nil {
		return nil, errors.New("nil link")
	}
	if ctx == nil {
		return nil, errors.New("nil context")
	}
	cfg = normalizeScanConfig(cfg)
	seen := make(map[cemi.IndividualAddr]struct{}, len(addrs))
	unique := make([]cemi.IndividualAddr, 0, len(addrs))
	for _, addr := range addrs {
		if addr == 0 {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		unique = append(unique, addr)
	}
	s := &scanner{
		ctx:       ctx,
		link:      link,
		cfg:       cfg,
		total:     len(unique),
		wasProbed: map[cemi.IndividualAddr]bool{},
		queued:    map[cemi.IndividualAddr]bool{},
		finished:  map[cemi.IndividualAddr]bool{},
	}
	if len(unique) == 0 {
		return nil, nil
	}

	var lastProbe time.Time
	for i, addr := range unique {
		if s.ctx.Err() != nil || s.stopped || s.fatal != nil {
			break
		}
		if !s.emit(AddressScanEvent{Phase: "probe", Address: addr, Index: i + 1, Total: s.total, Found: s.found}) {
			s.stopped = true
			break
		}
		s.wasProbed[addr] = true
		if err := s.send(addr, &cemi.ControlData{Command: 0}); err != nil {
			if fatalLink(err) {
				s.fatal = err
				break
			}
			// EMI1 的负确认只表示本机没收到 L2 应答。设备仍可能已经收下
			// T_Connect，大约 6 秒后还会发 T_Disconnect。这个地址要继续等。
			if !errors.Is(err, ErrLDataNegativeConfirm) {
				delete(s.wasProbed, addr)
				continue
			}
		}
		lastProbe = time.Now()
		s.drainFor(s.cfg.Interval)
		s.drainQueue()
	}

	if s.ctx.Err() == nil && !s.stopped && s.fatal == nil && !lastProbe.IsZero() {
		s.emit(AddressScanEvent{Phase: "wait", Index: s.total, Total: s.total, Found: s.found})
		deadline := lastProbe.Add(s.cfg.DisconnectWait)
		for s.ctx.Err() == nil && !s.stopped && s.fatal == nil {
			s.drainQueue()
			if s.fatal != nil || s.ctx.Err() != nil {
				break
			}
			if !time.Now().Before(deadline) {
				break
			}
			s.readOne(time.Until(deadline))
		}
	}
	s.drainQueue()

	sort.Slice(s.results, func(i, j int) bool { return s.results[i].Address < s.results[j].Address })
	if s.fatal != nil {
		return s.results, s.fatal
	}
	if err := s.ctx.Err(); err != nil {
		return s.results, err
	}
	return s.results, nil
}

// ReadDevices 对已知物理地址直接读掩码和序列号，不再发探测、也不等待设备自行断开。
func ReadDevices(ctx context.Context, link Link, addrs []cemi.IndividualAddr, cfg AddressScanConfig) ([]LineDevice, error) {
	if link == nil {
		return nil, errors.New("nil link")
	}
	if ctx == nil {
		return nil, errors.New("nil context")
	}
	cfg = normalizeScanConfig(cfg)
	seen := make(map[cemi.IndividualAddr]struct{}, len(addrs))
	unique := make([]cemi.IndividualAddr, 0, len(addrs))
	for _, addr := range addrs {
		if addr == 0 {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		unique = append(unique, addr)
	}
	if len(unique) == 0 {
		return nil, nil
	}
	s := &scanner{
		ctx:       ctx,
		link:      link,
		cfg:       cfg,
		total:     len(unique),
		wasProbed: map[cemi.IndividualAddr]bool{},
		queued:    map[cemi.IndividualAddr]bool{},
		finished:  map[cemi.IndividualAddr]bool{},
	}
	for i, addr := range unique {
		if s.ctx.Err() != nil || s.stopped || s.fatal != nil {
			break
		}
		if !s.emit(AddressScanEvent{Phase: "read", Address: addr, Index: i + 1, Total: s.total, Found: len(s.results)}) {
			s.stopped = true
			break
		}
		dev := s.readKnown(addr)
		s.results = append(s.results, dev)
		copied := dev
		s.emit(AddressScanEvent{Phase: "device", Address: addr, Index: i + 1, Total: s.total, Found: len(s.results), Device: &copied})
	}
	if s.fatal != nil {
		return s.results, s.fatal
	}
	if err := s.ctx.Err(); err != nil {
		return s.results, err
	}
	return s.results, nil
}

func normalizeScanConfig(cfg AddressScanConfig) AddressScanConfig {
	if cfg.Interval <= 0 {
		cfg.Interval = 60 * time.Millisecond
	}
	if cfg.DisconnectWait <= 0 {
		cfg.DisconnectWait = 7 * time.Second
	}
	if cfg.ReadTimeout <= 0 {
		cfg.ReadTimeout = 2 * time.Second
	}
	if cfg.ConnectSettle <= 0 {
		cfg.ConnectSettle = 30 * time.Millisecond
	}
	if cfg.RetryGap <= 0 {
		cfg.RetryGap = readRetryGap
	}
	if cfg.ReadAttempts <= 0 {
		cfg.ReadAttempts = readAttempts
	}
	return cfg
}

type scanner struct {
	ctx      context.Context
	link     Link
	cfg      AddressScanConfig
	total    int
	found    int
	stopped  bool
	fatal    error
	linkDead bool

	wasProbed map[cemi.IndividualAddr]bool
	queued    map[cemi.IndividualAddr]bool
	finished  map[cemi.IndividualAddr]bool
	queue     []cemi.IndividualAddr

	reading     cemi.IndividualAddr
	readAborted bool
	hit         *cemi.AppData
	hitSrc      cemi.IndividualAddr

	results []LineDevice
}

func (s *scanner) emit(ev AddressScanEvent) bool {
	if s.cfg.Progress == nil {
		return true
	}
	return s.cfg.Progress(ev)
}

func (s *scanner) drainQueue() {
	for s.ctx.Err() == nil && s.fatal == nil && len(s.queue) > 0 {
		s.readNext()
	}
}

func (s *scanner) readNext() {
	addr := s.queue[0]
	s.queue = s.queue[1:]
	s.finished[addr] = true
	dev := s.readKnown(addr)
	s.results = append(s.results, dev)
	copied := dev
	s.emit(AddressScanEvent{Phase: "device", Address: addr, Index: s.total, Total: s.total, Found: s.found, Device: &copied})
}

func (s *scanner) readKnown(addr cemi.IndividualAddr) LineDevice {
	dev := LineDevice{Address: addr}
	for attempt := 0; attempt < s.cfg.ReadAttempts; attempt++ {
		if s.ctx.Err() != nil || s.fatal != nil {
			break
		}
		if attempt > 0 {
			s.drainFor(s.cfg.RetryGap)
			if s.ctx.Err() != nil || s.fatal != nil {
				break
			}
		}
		s.reading = addr
		s.readAborted = false
		got, again := s.readDevice(addr)
		s.reading = 0
		if got.HasMask {
			dev = got
		}
		if !again || len(dev.Serial) > 0 {
			break
		}
	}
	return dev
}

func (s *scanner) drainFor(d time.Duration) {
	if d < 0 {
		d = 0
	}
	deadline := time.Now().Add(d)
	for {
		if s.ctx.Err() != nil || s.fatal != nil || s.linkDead {
			return
		}
		remain := time.Until(deadline)
		if remain <= 0 {
			return
		}
		if !s.readOne(remain) {
			return
		}
	}
}

// readOne 读取一帧。返回 false 表示超时、取消或链路关闭。
func (s *scanner) readOne(wait time.Duration) bool {
	if s.ctx.Err() != nil || s.linkDead {
		return false
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-s.ctx.Done():
		return false
	case <-timer.C:
		return false
	case msg, ok := <-s.link.Inbound():
		if !ok {
			s.linkDead = true
			s.fatal = errors.New("cEMI connection is closed")
			return false
		}
		s.observe(msg)
		return true
	}
}

func (s *scanner) observe(msg cemi.Message) {
	s.hit = nil
	var ld cemi.LData
	ind := false
	switch m := msg.(type) {
	case *cemi.LDataInd:
		ld = m.LData
		ind = true
	case *cemi.LDataCon:
		ld = m.LData
	default:
		return
	}
	if ld.Control2.IsGroupAddr() || !ind {
		return
	}
	src := ld.Source
	switch unit := ld.Data.(type) {
	case *cemi.ControlData:
		if !unit.Numbered && unit.Command == 1 {
			s.noteDisconnect(src)
		}
		if src == s.reading && unit.Numbered && unit.Command == 3 {
			s.readAborted = true
		}
	case *cemi.AppData:
		s.hit = unit
		s.hitSrc = src
	}
}

func (s *scanner) noteDisconnect(src cemi.IndividualAddr) {
	if src == s.reading {
		s.readAborted = true
		return
	}
	if !s.wasProbed[src] || s.queued[src] || s.finished[src] {
		return
	}
	s.queued[src] = true
	s.queue = append(s.queue, src)
	s.found++
}

// readDevice 读一次掩码和序列号。第二个返回值表示这次没读全，外层可以再试。
// 取消、链路断开，或设备明确拒绝序列号属性时不再试。
func (s *scanner) readDevice(addr cemi.IndividualAddr) (LineDevice, bool) {
	dev := LineDevice{Address: addr}
	defer func() {
		_ = s.send(addr, &cemi.ControlData{Command: 1})
		s.drainFor(20 * time.Millisecond)
	}()

	if err := s.send(addr, &cemi.ControlData{Command: 0}); fatalLink(err) {
		s.fatal = err
		return dev, false
	}
	s.drainFor(s.cfg.ConnectSettle)
	// 设备刚因连接超时拆链，紧接着的 T_Connect 偶尔会再收到一次 T_Disconnect。重试一次。
	if s.readAborted && s.ctx.Err() == nil && s.fatal == nil {
		s.readAborted = false
		if err := s.send(addr, &cemi.ControlData{Command: 0}); fatalLink(err) {
			s.fatal = err
			return dev, false
		}
		s.drainFor(s.cfg.ConnectSettle)
	}
	if s.ctx.Err() != nil || s.fatal != nil || s.readAborted {
		return dev, s.canRetry()
	}

	if err := s.send(addr, &cemi.AppData{Numbered: true, SeqNumber: 0, Command: cemi.MaskVersionRead, Data: []byte{0}}); err != nil {
		if fatalLink(err) {
			s.fatal = err
		}
		return dev, s.canRetry()
	}
	data, err := s.waitApp(addr, 0, 0x340)
	if err != nil {
		return dev, s.canRetry()
	}
	if len(data) >= 3 {
		dev.MaskVersion = uint16(data[1])<<8 | uint16(data[2])
		dev.HasMask = true
	}
	if !dev.HasMask {
		return dev, s.canRetry()
	}
	_ = s.send(addr, &cemi.ControlData{Numbered: true, SeqNumber: 0, Command: 2})

	serial, rejected := s.readSerial(addr, 1, pidDeviceSerial)
	if rejected {
		serial, rejected = s.readSerial(addr, 2, pidSerialNumberAlt)
	}
	dev.Serial = serial
	if len(serial) > 0 || rejected {
		return dev, false
	}
	return dev, s.canRetry()
}

func (s *scanner) canRetry() bool {
	return s.ctx.Err() == nil && s.fatal == nil
}

// readSerial 读一个序列号属性。第二个返回值表示设备明确拒绝了这次读（应答计数为 0），
// 只有这时才换一个属性号。超时留给外层整段重读。
func (s *scanner) readSerial(addr cemi.IndividualAddr, seq, pid uint8) ([]byte, bool) {
	payload := []byte{0, pid, 0x10, 0x01}
	app := &cemi.AppData{
		Numbered:  true,
		SeqNumber: seq,
		Command:   cemi.Escape,
		Data:      append([]byte{apciPropertyValueRead & 0x3f}, payload...),
	}
	if err := s.send(addr, app); err != nil {
		if fatalLink(err) {
			s.fatal = err
		}
		return nil, false
	}
	data, err := s.waitApp(addr, seq, apciPropertyValueResponse)
	if err != nil || len(data) < 4 || data[0] != 0 || data[1] != pid {
		return nil, false
	}
	_ = s.send(addr, &cemi.ControlData{Numbered: true, SeqNumber: seq, Command: 2})
	if data[2]>>4 == 0 {
		return nil, true
	}
	return append([]byte(nil), data[4:]...), false
}

func (s *scanner) waitApp(addr cemi.IndividualAddr, seq uint8, expect uint16) ([]byte, error) {
	deadline := time.Now().Add(s.cfg.ReadTimeout)
	for {
		if err := s.ctx.Err(); err != nil {
			return nil, err
		}
		if s.fatal != nil {
			return nil, s.fatal
		}
		if s.readAborted {
			return nil, errors.New("device disconnected")
		}
		remain := time.Until(deadline)
		if remain <= 0 {
			return nil, errors.New("response timeout")
		}
		if !s.readOne(remain) {
			if err := s.ctx.Err(); err != nil {
				return nil, err
			}
			if s.fatal != nil {
				return nil, s.fatal
			}
			if s.readAborted {
				return nil, errors.New("device disconnected")
			}
			return nil, errors.New("response timeout")
		}
		if data, ok := s.takeMatch(addr, seq, expect); ok {
			return data, nil
		}
	}
}

func (s *scanner) takeMatch(addr cemi.IndividualAddr, seq uint8, expect uint16) ([]byte, bool) {
	app := s.hit
	src := s.hitSrc
	s.hit = nil
	if app == nil || src != addr {
		return nil, false
	}
	if app.Numbered && app.SeqNumber != seq {
		return nil, false
	}
	got := apciOf(app)
	if app.Command == cemi.Escape {
		if got != expect || len(app.Data) == 0 {
			return nil, false
		}
		return append([]byte(nil), app.Data[1:]...), true
	}
	if got&0x3c0 != expect&0x3c0 {
		return nil, false
	}
	return append([]byte(nil), app.Data...), true
}

func (s *scanner) send(addr cemi.IndividualAddr, unit cemi.TransportUnit) error {
	ctrl := cemi.Control1NoRepeat | cemi.Control1NoSysBroadcast | cemi.Control1Prio(cemi.PrioSystem)
	// Same rule as Device.send: standard frame when the NPDU fits in 15 octets.
	if unit.Size() <= 17 {
		ctrl |= cemi.Control1StdFrame
	}
	return s.link.Send(&cemi.LDataReq{LData: cemi.LData{
		Control1:    ctrl,
		Control2:    cemi.Control2Hops(6),
		Destination: uint16(addr),
		Data:        unit,
	}})
}

func apciOf(app *cemi.AppData) uint16 {
	if len(app.Data) == 0 {
		return 0xffff
	}
	return uint16(app.Command)<<6 | uint16(app.Data[0]&0x3f)
}

func fatalLink(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "closed") || strings.Contains(msg, "nil cEMI")
}
