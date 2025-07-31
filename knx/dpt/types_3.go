package dpt

import (
	"fmt"
	"strconv"
)

type DPT_3007 uint8

func (d DPT_3007) Pack() []byte {
	return packU8(uint8(d))
}

func (d *DPT_3007) Unpack(data []byte) error {
	return unpackU8(data, (*uint8)(d))
}

func (d DPT_3007) Unit() string {
	return ""
}

func (d DPT_3007) String() string {
	return fmt.Sprintf("%d", uint8(d))
}
func (d DPT_3007) Float() float64 {
	return float64(d)
}
func (d *DPT_3007) ToByteArray(data string) ([]byte, error) {
	result, err := strconv.ParseFloat(data, 32)
	if err != nil {
		return nil, err
	}
	*d = DPT_3007(result)
	return d.Pack(), nil
}
