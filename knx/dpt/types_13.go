// Copyright 2017 Ole Krüger.
// Licensed under the MIT license which can be found in the LICENSE file.

package dpt

import (
	"fmt"
	"strconv"
)

// DPT_13001 represents DPT 13.001 / counter value (pulses).
type DPT_13001 int32

func (d DPT_13001) Pack() []byte {
	return packV32(int32(d))
}

func (d *DPT_13001) Unpack(data []byte) error {
	return unpackV32(data, (*int32)(d))
}

func (d DPT_13001) Unit() string {
	return "pulses"
}

func (d DPT_13001) String() string {
	return fmt.Sprintf("%d", int32(d))
}
func (d DPT_13001) Float() float64 {
	return float64(d)
}
func (d *DPT_13001) ToByteArray(data string) ([]byte, error) {
	result, err := strconv.Atoi(data)
	if err != nil {
		return nil, err
	}
	*d = DPT_13001(result)
	return d.Pack(), nil
}

// DPT_13002 represents DPT 13.002 / flow rate (m^3/h).
type DPT_13002 int32

func (d DPT_13002) Pack() []byte {
	return packV32(int32(d))
}

func (d *DPT_13002) Unpack(data []byte) error {
	return unpackV32(data, (*int32)(d))
}

func (d DPT_13002) Unit() string {
	return "m^3/h"
}

func (d DPT_13002) String() string {
	return fmt.Sprintf("%d", int32(d))
}
func (d DPT_13002) Float() float64 {
	return float64(d)
}
func (d *DPT_13002) ToByteArray(data string) ([]byte, error) {
	result, err := strconv.Atoi(data)
	if err != nil {
		return nil, err
	}
	*d = DPT_13002(result)
	return d.Pack(), nil
}

// DPT_13010 represents DPT 13.010 / active energy (Wh).
type DPT_13010 int32

func (d DPT_13010) Pack() []byte {
	return packV32(int32(d))
}

func (d *DPT_13010) Unpack(data []byte) error {
	return unpackV32(data, (*int32)(d))
}

func (d DPT_13010) Unit() string {
	return "Wh"
}

func (d DPT_13010) String() string {
	return fmt.Sprintf("%d", int32(d))
}
func (d DPT_13010) Float() float64 {
	return float64(d)
}
func (d *DPT_13010) ToByteArray(data string) ([]byte, error) {
	result, err := strconv.Atoi(data)
	if err != nil {
		return nil, err
	}
	*d = DPT_13010(result)
	return d.Pack(), nil
}

// DPT_13011 represents DPT 13.011 / apparant energy (VAh).
type DPT_13011 int32

func (d DPT_13011) Pack() []byte {
	return packV32(int32(d))
}

func (d *DPT_13011) Unpack(data []byte) error {
	return unpackV32(data, (*int32)(d))
}

func (d DPT_13011) Unit() string {
	return "VAh"
}

func (d DPT_13011) String() string {
	return fmt.Sprintf("%d", int32(d))
}
func (d DPT_13011) Float() float64 {
	return float64(d)
}
func (d *DPT_13011) ToByteArray(data string) ([]byte, error) {
	result, err := strconv.Atoi(data)
	if err != nil {
		return nil, err
	}
	*d = DPT_13011(result)
	return d.Pack(), nil
}

// DPT_13012 represents DPT 13.012 / reactive energy (VARh).
type DPT_13012 int32

func (d DPT_13012) Pack() []byte {
	return packV32(int32(d))
}

func (d *DPT_13012) Unpack(data []byte) error {
	return unpackV32(data, (*int32)(d))
}

func (d DPT_13012) Unit() string {
	return "VARh"
}

func (d DPT_13012) String() string {
	return fmt.Sprintf("%d", int32(d))
}
func (d DPT_13012) Float() float64 {
	return float64(d)
}
func (d *DPT_13012) ToByteArray(data string) ([]byte, error) {
	result, err := strconv.Atoi(data)
	if err != nil {
		return nil, err
	}
	*d = DPT_13012(result)
	return d.Pack(), nil
}

// DPT_13013 represents DPT 13.013 / active energy (kWh).
type DPT_13013 int32

func (d DPT_13013) Pack() []byte {
	return packV32(int32(d))
}

func (d *DPT_13013) Unpack(data []byte) error {
	return unpackV32(data, (*int32)(d))
}

func (d DPT_13013) Unit() string {
	return "kWh"
}

func (d DPT_13013) String() string {
	return fmt.Sprintf("%d", int32(d))
}
func (d DPT_13013) Float() float64 {
	return float64(d)
}
func (d *DPT_13013) ToByteArray(data string) ([]byte, error) {
	result, err := strconv.Atoi(data)
	if err != nil {
		return nil, err
	}
	*d = DPT_13013(result)
	return d.Pack(), nil
}

// DPT_13014 represents DPT 13.014 / apparant energy (kVAh).
type DPT_13014 int32

func (d DPT_13014) Pack() []byte {
	return packV32(int32(d))
}

func (d *DPT_13014) Unpack(data []byte) error {
	return unpackV32(data, (*int32)(d))
}

func (d DPT_13014) Unit() string {
	return "kVAh"
}

func (d DPT_13014) String() string {
	return fmt.Sprintf("%d", int32(d))
}
func (d DPT_13014) Float() float64 {
	return float64(d)
}
func (d *DPT_13014) ToByteArray(data string) ([]byte, error) {
	result, err := strconv.Atoi(data)
	if err != nil {
		return nil, err
	}
	*d = DPT_13014(result)
	return d.Pack(), nil
}

// DPT_13015 represents DPT 13.015 / reactive energy (kVARh).
type DPT_13015 int32

func (d DPT_13015) Pack() []byte {
	return packV32(int32(d))
}

func (d *DPT_13015) Unpack(data []byte) error {
	return unpackV32(data, (*int32)(d))
}

func (d DPT_13015) Unit() string {
	return "kVARh"
}

func (d DPT_13015) String() string {
	return fmt.Sprintf("%d", int32(d))
}
func (d DPT_13015) Float() float64 {
	return float64(d)
}
func (d *DPT_13015) ToByteArray(data string) ([]byte, error) {
	result, err := strconv.Atoi(data)
	if err != nil {
		return nil, err
	}
	*d = DPT_13015(result)
	return d.Pack(), nil
}

// DPT_13016 represents DPT 13.016 / apparant energy (MWh).
type DPT_13016 int32

func (d DPT_13016) Pack() []byte {
	return packV32(int32(d))
}

func (d *DPT_13016) Unpack(data []byte) error {
	return unpackV32(data, (*int32)(d))
}

func (d DPT_13016) Unit() string {
	return "MWh"
}

func (d DPT_13016) String() string {
	return fmt.Sprintf("%d", int32(d))
}
func (d DPT_13016) Float() float64 {
	return float64(d)
}
func (d *DPT_13016) ToByteArray(data string) ([]byte, error) {
	result, err := strconv.Atoi(data)
	if err != nil {
		return nil, err
	}
	*d = DPT_13016(result)
	return d.Pack(), nil
}

// DPT_13100 represents DPT 13.100 / delta time (s).
type DPT_13100 int32

func (d DPT_13100) Pack() []byte {
	return packV32(int32(d))
}

func (d *DPT_13100) Unpack(data []byte) error {
	return unpackV32(data, (*int32)(d))
}

func (d DPT_13100) Unit() string {
	return "s"
}

func (d DPT_13100) String() string {
	return fmt.Sprintf("%d", int32(d))
}
func (d DPT_13100) Float() float64 {
	return float64(d)
}
func (d *DPT_13100) ToByteArray(data string) ([]byte, error) {
	result, err := strconv.Atoi(data)
	if err != nil {
		return nil, err
	}
	*d = DPT_13100(result)
	return d.Pack(), nil
}
