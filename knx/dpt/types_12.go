package dpt

import (
	"fmt"
	"strconv"
)

// DPT_12001 represents DPT 12.001 / Unsigned counter.
type DPT_12001 uint32

func (d DPT_12001) Pack() []byte {
	return packU32(uint32(d))
}

func (d *DPT_12001) Unpack(data []byte) error {
	return unpackU32(data, (*uint32)(d))
}

func (d DPT_12001) Unit() string {
	return "pulses"
}

func (d DPT_12001) String() string {
	return fmt.Sprintf("%d", uint32(d))
}
func (d DPT_12001) Float() float64 {
	return float64(d)
}
func (d *DPT_12001) ToByteArray(data string) ([]byte, error) {
	result, err := strconv.Atoi(data)
	if err != nil {
		return nil, err
	}
	*d = DPT_12001(result)
	return d.Pack(), nil
}
