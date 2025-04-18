package dpt

import (
	"fmt"
	"strconv"
	"strings"
)

// DPT_232600 represents DPT 232.600 (G) / DPT_Colour_RGB.
// Colour RGB - RGB value 4x(0..100%) / U8 U8 U8
type DPT_232600 struct {
	Red   uint8
	Green uint8
	Blue  uint8
}

func (d DPT_232600) Pack() []byte {
	return []byte{0, d.Red, d.Green, d.Blue}
}

func (d *DPT_232600) Unpack(data []byte) error {

	if len(data) != 4 {
		return ErrInvalidLength
	}

	d.Red = data[1]
	d.Green = data[2]
	d.Blue = data[3]

	return nil
}

func (d DPT_232600) Unit() string {
	return ""
}

func (d DPT_232600) String() string {
	return fmt.Sprintf("%d,%d,%d", d.Red, d.Green, d.Blue)
}
func (d DPT_232600) Float() float64 {
	return float64(d.Red) + float64(d.Green) + float64(d.Blue)
}
func (d *DPT_232600) ToByteArray(data string) ([]byte, error) {
	split := strings.Split(data, ",")
	if len(split) != 3 {
		return nil, ErrInvalidLength
	}
	red, err := strconv.Atoi(split[0])
	if err != nil {
		return nil, err
	}
	d.Red = uint8(red)
	green, err := strconv.Atoi(split[1])
	if err != nil {
		return nil, err
	}
	d.Green = uint8(green)
	blue, err := strconv.Atoi(split[2])
	if err != nil {
		return nil, err
	}
	d.Blue = uint8(blue)
	return d.Pack(), nil
}
