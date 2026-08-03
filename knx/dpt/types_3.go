package dpt

import (
	"fmt"
	"strconv"
)

// DPT_3007 代表 KNX DPT 3.007 (3-Bit controlled / Dimming Control)
// 存储的值为 0~15 的十进制整数
type DPT_3007 uint8

// Pack 将 DPT_3007 打包为单字节数组
func (d DPT_3007) Pack() []byte {
	// 确保数据安全，只保留 4 位有效数据 (0x0F)
	val := uint8(d) & 0x0F
	return []byte{val}
}

// Unpack 从字节数组中解包恢复出 DPT_3007 的数值
func (d *DPT_3007) Unpack(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("dpt 3.007: data is empty")
	}
	// 取出第一个字节，并过滤高4位干扰，只保留 4-bit 原始值
	*d = DPT_3007(data[0] & 0x0F)
	return nil
}

// Unit 返回该数据类型的物理单位 (相对调光没有固定物理单位，返回空字符串)
func (d DPT_3007) Unit() string {
	return ""
}

// String 将数值转换为易读的 ETS 风格格式
func (d DPT_3007) String() string {
	val := uint8(d) & 0x0F
	isIncrease := (val & 0x08) != 0 // 第3位判断方向
	stepCode := val & 0x07          // 低3位判断步长

	if stepCode == 0 {
		if isIncrease {
			return "Increase, Break"
		}
		return "Decrease, Break"
	}

	// 映射 StepCode 到 ETS 常见的相对百分比
	intervals := []string{"100%", "50%", "25%", "12.5%", "6.25%", "3.125%", "1.5625%"}
	percent := intervals[stepCode-1]

	if isIncrease {
		return fmt.Sprintf("Increase, %s", percent)
	}
	return fmt.Sprintf("Decrease, %s", percent)
}

// Float 返回数值的浮点数形式 (方便上层框架做统一数值处理)
func (d DPT_3007) Float() float64 {
	return float64(uint8(d) & 0x0F)
}

// ToByteArray 将上层传入的字符串数据(如 "12")解析并赋值给结构体，最后返回打包后的字节数组
func (d *DPT_3007) ToByteArray(data string) ([]byte, error) {
	result, err := strconv.ParseFloat(data, 64)
	if err != nil {
		return nil, err
	}

	val := uint8(result)
	if val > 15 {
		return nil, fmt.Errorf("dpt 3.007: value %d out of range [0..15]", val)
	}

	*d = DPT_3007(val)
	return d.Pack(), nil
}
