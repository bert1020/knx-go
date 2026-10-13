// Copyright 2017 Ole Krüger.
// Licensed under the MIT license which can be found in the LICENSE file.

package dpt

import (
	"fmt"
	"testing"
)

// Test DPT 1.xxx (B1)
func TestDPT_1(t *testing.T) {
	type DPT1 struct {
		Dpv     DatapointValue
		OnFalse string
		OnTrue  string
	}

	var types_1 = []DPT1{
		{new(DPT_1001), "0", "1"},
		{new(DPT_1002), "0", "1"},
		{new(DPT_1003), "0", "1"},
		{new(DPT_1004), "0", "1"},
		{new(DPT_1005), "0", "1"},
		{new(DPT_1006), "0", "1"},
		{new(DPT_1007), "0", "1"},
		{new(DPT_1008), "0", "1"},
		{new(DPT_1009), "0", "1"},
		{new(DPT_1010), "0", "1"},
		{new(DPT_1011), "0", "1"},
		{new(DPT_1012), "0", "1"},
		{new(DPT_1013), "0", "1"},
		{new(DPT_1014), "0", "1"},
		{new(DPT_1015), "0", "1"},
		{new(DPT_1016), "0", "1"},
		{new(DPT_1017), "0", "1"},
		{new(DPT_1018), "0", "1"},
		{new(DPT_1019), "0", "1"},
		{new(DPT_1021), "0", "1"},
		{new(DPT_1022), "0", "1"},
		{new(DPT_1023), "0", "1"},
		{new(DPT_1024), "0", "1"},
		{new(DPT_1100), "0", "1"}}

	for _, e := range types_1 {
		src := e.Dpv
		if fmt.Sprintf("%s", src) != e.OnFalse {
			t.Errorf("%#v has wrong default value [%v]. Should be [%s].", e.Dpv, e.Dpv, e.OnFalse)
		}

		e.Dpv.Unpack([]byte{packB1(false)})
		if fmt.Sprintf("%s", e.Dpv) != e.OnFalse {
			t.Errorf("%#v has wrong false value [%v]. Should be [%s].", e.Dpv, e.Dpv, e.OnFalse)
		}

		e.Dpv.Unpack([]byte{packB1(true)})
		if fmt.Sprintf("%s", e.Dpv) != e.OnTrue {
			t.Errorf("%#v has wrong true value [%v]. Should be [%s].", e.Dpv, e.Dpv, e.OnTrue)
		}
	}
}

// Test DPT 1.001 (Switch) with values within range
func TestDPT_1001(t *testing.T) {
	var buf []byte
	var src, dst DPT_1001

	for _, value := range []bool{true, false} {
		src = DPT_1001(value)
		if bool(src) != value {
			t.Errorf("Assignment of value \"%v\" failed! Has value \"%s\".", value, src)
		}
		buf = src.Pack()
		dst.Unpack(buf)
		if bool(dst) != value {
			t.Errorf("Wrong value \"%s\" after pack/unpack! Original value was \"%v\".", dst, value)
		}
	}
}

// Test DPT 1.002 (Bool) with values within range
func TestDPT_1002(t *testing.T) {
	var buf []byte
	var src, dst DPT_1002

	for _, value := range []bool{true, false} {
		src = DPT_1002(value)
		if bool(src) != value {
			t.Errorf("Assignment of value \"%v\" failed! Has value \"%s\".", value, src)
		}
		buf = src.Pack()
		dst.Unpack(buf)
		if bool(dst) != value {
			t.Errorf("Wrong value \"%s\" after pack/unpack! Original value was \"%v\".", dst, value)
		}
	}
}
func TestDPT_1001_Float(t *testing.T) {
	d, _ := Produce("9.001")
	array, err := d.ToByteArray("25.5")
	if err != nil {
		t.Error(err)

	}
	err = d.Unpack(array)
	if err != nil {
		t.Error(err)
	}
	fmt.Println(d.String())
}

// Test DPT 1.003 (Enable) with values within range
func TestDPT_1003(t *testing.T) {
	var buf []byte
	var src, dst DPT_1003

	for _, value := range []bool{true, false} {
		src = DPT_1003(value)
		if bool(src) != value {
			t.Errorf("Assignment of value \"%v\" failed! Has value \"%s\".", value, src)
		}
		buf = src.Pack()
		dst.Unpack(buf)
		if bool(dst) != value {
			t.Errorf("Wrong value \"%s\" after pack/unpack! Original value was \"%v\".", dst, value)
		}
	}
}

// Test DPT 1.008 (Up/Down) with values within range
func TestDPT_1008(t *testing.T) {
	var buf []byte
	var src, dst DPT_1008

	for _, value := range []bool{true, false} {
		src = DPT_1008(value)
		if bool(src) != value {
			t.Errorf("Assignment of value \"%v\" failed! Has value \"%s\".", value, src)
		}
		buf = src.Pack()
		dst.Unpack(buf)
		if bool(dst) != value {
			t.Errorf("Wrong value \"%s\" after pack/unpack! Original value was \"%v\".", dst, value)
		}
	}
}

// Test DPT 1.009 (OpenClose) with values within range
func TestDPT_1009(t *testing.T) {
	var buf []byte
	var src, dst DPT_1009

	for _, value := range []bool{true, false} {
		src = DPT_1009(value)
		if bool(src) != value {
			t.Errorf("Assignment of value \"%v\" failed! Has value \"%s\".", value, src)
		}
		buf = src.Pack()
		dst.Unpack(buf)
		if bool(dst) != value {
			t.Errorf("Wrong value \"%s\" after pack/unpack! Original value was \"%v\".", dst, value)
		}
	}
}

// Test DPT 1.010 (Start) with values within range
func TestDPT_1010(t *testing.T) {
	var buf []byte
	var src, dst DPT_1010

	for _, value := range []bool{true, false} {
		src = DPT_1010(value)
		if bool(src) != value {
			t.Errorf("Assignment of value \"%v\" failed! Has value \"%s\".", value, src)
		}
		buf = src.Pack()
		dst.Unpack(buf)
		if bool(dst) != value {
			t.Errorf("Wrong value \"%s\" after pack/unpack! Original value was \"%v\".", dst, value)
		}
	}
}

// Test DPT 1.100 (Heat/Cool) with values within range
func TestDPT_1100(t *testing.T) {
	var buf []byte
	var src, dst DPT_1100

	for _, value := range []bool{true, false} {
		src = DPT_1100(value)
		if bool(src) != value {
			t.Errorf("Assignment of value \"%v\" failed! Has value \"%s\".", value, src)
		}
		buf = src.Pack()
		dst.Unpack(buf)
		if bool(dst) != value {
			t.Errorf("Wrong value \"%s\" after pack/unpack! Original value was \"%v\".", dst, value)
		}
	}
}