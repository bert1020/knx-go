package dpt

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type DPT_19001 struct {
	Year    uint16
	Month   uint8
	Day     uint8
	Weekday uint8 // 1-7 (Mon-Sun), 0 为未定义
	Hour    uint8
	Minute  uint8
	Second  uint8
	// 状态位
	Fault   bool // 数据是否损坏
	Working bool // 只有时间有效 (Date无效)
	Summer  bool // 是否夏令时
}

func (d DPT_19001) Pack() []byte {
	var buf = make([]byte, 9) // KNX Data 为 8 字节，加 1 字节控制位（根据框架实现可能只需要8字节）
	// 这里按标准 8 字节数据负载处理 (data[1]-data[8])

	if d.Year >= 1900 && d.Year <= 2155 {
		buf[1] = uint8(d.Year - 1900)
		buf[2] = d.Month & 0x0F
		buf[3] = d.Day & 0x1F

		// Byte 4: Day of week (5-7 bit) + Hour (0-4 bit)
		wd := d.Weekday
		if wd == 7 {
			wd = 0
		} // 部分实现中 0 代表周日，此处按标准映射
		buf[4] = (d.Weekday << 5) | (d.Hour & 0x1F)

		buf[5] = d.Minute & 0x3F
		buf[6] = d.Second & 0x3F

		// Byte 7 & 8 状态位处理
		if d.Fault {
			buf[7] |= 0x80
		}
		if d.Working {
			buf[7] |= 0x40
		}
		if d.Summer {
			buf[8] |= 0x01
		}
	}
	return buf
}
func (d DPT_19001) Unit() string {
	return ""
}
func (d DPT_19001) Float() float64 {
	return 0
}
func (d *DPT_19001) Unpack(data []byte) error {
	if len(data) < 9 {
		return fmt.Errorf("invalid length")
	}

	d.Year = uint16(data[1]) + 1900
	d.Month = data[2] & 0x0F
	d.Day = data[3] & 0x1F
	d.Weekday = (data[4] >> 5) & 0x07
	d.Hour = data[4] & 0x1F
	d.Minute = data[5] & 0x3F
	d.Second = data[6] & 0x3F

	d.Fault = (data[7] & 0x80) != 0
	d.Working = (data[7] & 0x40) != 0
	d.Summer = (data[8] & 0x01) != 0

	if !d.IsValid() {
		return fmt.Errorf("payload is out of range")
	}
	return nil
}

func (d DPT_19001) IsValid() bool {
	if d.Month < 1 || d.Month > 12 || d.Day < 1 || d.Day > 31 || d.Hour > 23 || d.Minute > 59 || d.Second > 59 {
		return false
	}
	return true
}

func (d DPT_19001) String() string {
	return fmt.Sprintf("%04d-%02d-%02d %02d:%02d:%02d", d.Year, d.Month, d.Day, d.Hour, d.Minute, d.Second)
}

// ToByteArray 将 "2026,1,9,18,30,0" 转为字节
func (d *DPT_19001) ToByteArray(data string) ([]byte, error) {
	if data == "" {
		now := time.Now()
		d.Year = uint16(now.Year())
		d.Month = uint8(now.Month())
		d.Day = uint8(now.Day())
		// Go 的 Weekday 是 0(Sun)-6(Sat)，KNX 19.001 通常 1(Mon)-7(Sun)
		wd := int(now.Weekday())
		if wd == 0 {
			wd = 7
		}
		d.Weekday = uint8(wd)
		d.Hour = uint8(now.Hour())
		d.Minute = uint8(now.Minute())
		d.Second = uint8(now.Second())
	} else {
		split := strings.Split(data, ",")
		if len(split) < 6 {
			return nil, fmt.Errorf("invalid length")
		}
		y, _ := strconv.Atoi(split[0])
		d.Year = uint16(y)
		m, _ := strconv.Atoi(split[1])
		d.Month = uint8(m)
		day, _ := strconv.Atoi(split[2])
		d.Day = uint8(day)
		h, _ := strconv.Atoi(split[3])
		d.Hour = uint8(h)
		min, _ := strconv.Atoi(split[4])
		d.Minute = uint8(min)
		s, _ := strconv.Atoi(split[5])
		d.Second = uint8(s)

		// 自动计算星期
		tm := time.Date(int(d.Year), time.Month(d.Month), int(d.Day), int(d.Hour), int(d.Minute), int(d.Second), 0, time.Local)
		wd := int(tm.Weekday())
		if wd == 0 {
			wd = 7
		}
		d.Weekday = uint8(wd)
	}
	return d.Pack(), nil
}
