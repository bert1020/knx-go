// Copyright 2017 Ole Krüger.
// Licensed under the MIT license which can be found in the LICENSE file.

package dpt

import (
	"encoding/binary"
	"errors"
	"math"
)

// ErrInvalidLength is returned when the application data has unexpected length.
var ErrInvalidLength = errors.New("given application data has invalid length")

func packB1(b bool) []byte {
	if b {
		return []byte{1}
	}

	return []byte{0}
}

func unpackB1(data []byte, b *bool) error {
	if len(data) != 1 {
		return ErrInvalidLength
	}

	*b = data[0]&1 == 1

	return nil
}

func packF16(f float32) []byte {
	buffer := []byte{0, 0, 0}

	if f > 670760.96 {
		f = 670760.96
	} else if f < -671088.64 {
		f = -671088.64
	}

	signedMantissa := int(f * 100)
	exp := 0

	for signedMantissa > 2047 || signedMantissa < -2048 {
		signedMantissa /= 2
		exp++
	}

	buffer[1] |= uint8(exp&15) << 3

	if signedMantissa < 0 {
		signedMantissa += 2048
		buffer[1] |= 1 << 7
	}

	mantissa := uint(signedMantissa)

	buffer[1] |= uint8(mantissa>>8) & 7
	buffer[2] |= uint8(mantissa)

	return buffer
}

func unpackF16(data []byte, f *float32) error {
	if len(data) != 3 {
		return ErrInvalidLength
	}

	m := int(data[1]&7)<<8 | int(data[2])
	if data[1]&128 == 128 {
		m -= 2048
	}

	e := (data[1] >> 3) & 15

	*f = 0.01 * float32(m) * float32(uint(1)<<e)
	return nil
}

func packF32(f float32) []byte {
	buffer := []byte{0, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(buffer[1:], math.Float32bits(f))
	return buffer
}

func unpackF32(data []byte, f *float32) error {
	if len(data) != 5 {
		return ErrInvalidLength
	}
	*f = math.Float32frombits(binary.BigEndian.Uint32(data[1:]))
	return nil
}

func packU8(i uint8) []byte {
	return []byte{0, i}
}

func unpackU8(data []byte, i *uint8) error {
	if len(data) != 2 {
		return ErrInvalidLength
	}

	*i = uint8(data[1])

	return nil
}

func packU32(i uint32) []byte {
	buffer := []byte{0, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(buffer[1:], i)
	return buffer
}

func unpackU32(data []byte, i *uint32) error {
	if len(data) != 5 {
		return ErrInvalidLength
	}
	*i = binary.BigEndian.Uint32(data[1:])
	return nil
}

func packV32(i int32) []byte {
	b := make([]byte, 5)

	b[0] = 0
	b[1] = byte((i >> 24) & 0xff)
	b[2] = byte((i >> 16) & 0xff)
	b[3] = byte((i >> 8) & 0xff)
	b[4] = byte(i & 0xff)

	return b
}

func unpackV32(data []byte, i *int32) error {
	if len(data) != 5 {
		return ErrInvalidLength
	}

	*i = int32(data[1])<<24 | int32(data[2])<<16 | int32(data[3])<<8 | int32(data[4])

	return nil
}
