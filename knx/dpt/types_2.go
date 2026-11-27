package dpt

import (
	"fmt"
	"math"
	"strconv"
)

// DPT2 represents a KNX B2 value. Bit 1 is the control flag and bit 0
// is the function value. When the control flag is clear, no control is applied.
type DPT2 uint8

const dpt2Mask = 0x03

func (d DPT2) Pack() []byte {
	return []byte{byte(d) & dpt2Mask}
}

func (d *DPT2) Unpack(data []byte) error {
	if len(data) != 1 {
		return ErrInvalidLength
	}
	*d = DPT2(data[0] & dpt2Mask)
	return nil
}

func (d DPT2) Unit() string {
	return ""
}

func (d DPT2) String() string {
	return strconv.Itoa(int(d & dpt2Mask))
}

func (d DPT2) Float() float64 {
	return float64(d & dpt2Mask)
}

func (d *DPT2) ToByteArray(data string) ([]byte, error) {
	value, err := strconv.ParseFloat(data, 64)
	if err != nil {
		return nil, err
	}
	if value < 0 || value > dpt2Mask || value != math.Trunc(value) {
		return nil, fmt.Errorf("dpt 2.xxx: expected an integer from 0 to 3, got %q", data)
	}
	*d = DPT2(value)
	return d.Pack(), nil
}

// DPT_2001 represents DPT_Switch_Control.
type DPT_2001 = DPT2

// DPT_2002 represents DPT_Bool_Control.
type DPT_2002 = DPT2

// DPT_2003 represents DPT_Enable_Control.
type DPT_2003 = DPT2

// DPT_2004 represents DPT_Ramp_Control.
type DPT_2004 = DPT2

// DPT_2005 represents DPT_Alarm_Control.
type DPT_2005 = DPT2

// DPT_2006 represents DPT_BinaryValue_Control.
type DPT_2006 = DPT2

// DPT_2007 represents DPT_Step_Control.
type DPT_2007 = DPT2

// DPT_2008 represents DPT_Direction1_Control.
type DPT_2008 = DPT2

// DPT_2009 represents DPT_Direction2_Control.
type DPT_2009 = DPT2

// DPT_2010 represents DPT_Start_Control.
type DPT_2010 = DPT2

// DPT_2011 represents DPT_State_Control.
type DPT_2011 = DPT2

// DPT_2012 represents DPT_Invert_Control.
type DPT_2012 = DPT2
