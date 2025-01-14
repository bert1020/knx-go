package dpt

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// DPT_242600 represents DPT 242.600 (DPT_Colour_xyY)
// Colour xyY - x: 0-1 (= 0 - 65535) y: 0-1 (= 0 - 65535)
// U16 U16 U8 r6B2
type DPT_242600 struct {
	X               uint16
	Y               uint16
	YBrightness     uint8
	ColorValid      bool
	BrightnessValid bool
}

func (d DPT_242600) Pack() []byte {
	validBits := packB2([2]bool{d.ColorValid, d.BrightnessValid})

	x := packU16(uint16(d.X))
	y := packU16(uint16(d.Y))

	return []byte{0, x[1], x[2], y[1], y[2], d.YBrightness, validBits}
}

func (d *DPT_242600) Unpack(data []byte) error {
	if len(data) != 7 {
		return ErrInvalidLength
	}

	var colorValid, brightnessValid bool

	err := unpackB2(data[6], &colorValid, &brightnessValid)

	if err != nil {
		return errors.New("unpackB2: " + err.Error())
	}

	var x, y uint16

	xData := []byte{0}
	xData = append(xData, data[1], data[2])

	err = unpackU16(xData, &x)

	if err != nil {
		return errors.New("unpackU16 x: " + err.Error())
	}

	yData := []byte{0}
	yData = append(yData, data[3], data[4])

	err = unpackU16(yData, &y)

	if err != nil {
		return errors.New("unpackU16 y: " + err.Error())

	}

	*d = DPT_242600{
		X:               x,
		Y:               y,
		YBrightness:     uint8(data[5]),
		ColorValid:      colorValid,
		BrightnessValid: brightnessValid,
	}

	return nil
}

func (d DPT_242600) Unit() string {
	return ""
}

func (d DPT_242600) String() string {
	return fmt.Sprintf("x: %d y: %d Y: %d ColorValid: %t, BrightnessValid: %t", d.X, d.Y, d.YBrightness, d.ColorValid, d.BrightnessValid)
}
func (d DPT_242600) Float() float64 {
	return float64(d.X) + float64(d.Y) + float64(d.YBrightness)
}
func (d *DPT_242600) ToByteArray(data string) ([]byte, error) {
	split := strings.Split(data, ",")
	if len(split) != 5 {
		return nil, ErrInvalidLength
	}
	x, err := strconv.Atoi(split[0])
	if err != nil {
		return nil, err
	}
	d.X = uint16(x)
	y, err := strconv.Atoi(split[1])
	if err != nil {
		return nil, err
	}
	d.Y = uint16(y)
	YBrightness, err := strconv.Atoi(split[2])
	if err != nil {
		return nil, err
	}
	d.YBrightness = uint8(YBrightness)
	d.ColorValid = split[3] == "1" || strings.ToLower(split[3]) == "true"
	d.BrightnessValid = split[4] == "1" || strings.ToLower(split[4]) == "true"
	return d.Pack(), nil
}
