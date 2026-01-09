package dpt

import (
	"fmt"
	"testing"
	"time"
)

func TestDPT_19001(t *testing.T) {
	dpt19001 := DPT_19001{}
	dpt19001.Year = 2026
	dpt19001.Month = 1
	dpt19001.Day = 9
	wd := uint8(time.Now().Weekday())
	if wd == 0 {
		wd = 7
	}
	dpt19001.Weekday = wd
	dpt19001.Hour = uint8(15)
	dpt19001.Minute = uint8(01)
	dpt19001.Second = uint8(30)
	pack := dpt19001.Pack()
	fmt.Printf("% X\n", pack)

}
