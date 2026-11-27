package dpt

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDPT2RoundTrip(t *testing.T) {
	for subtype := 1; subtype <= 12; subtype++ {
		name := fmt.Sprintf("2.%03d", subtype)
		t.Run(name, func(t *testing.T) {
			d, ok := Produce(name)
			require.True(t, ok)
			for value := byte(0); value < 4; value++ {
				wire := []byte{value}
				decimal := strconv.Itoa(int(value))
				packed, err := d.ToByteArray(decimal + ".0")
				require.NoError(t, err)
				require.Equal(t, wire, packed)

				require.NoError(t, d.Unpack([]byte{value | 0xfc}))
				require.Equal(t, wire, d.Pack())
				require.Equal(t, decimal, d.String())
				require.Equal(t, float64(value), d.Float())
			}
		})
	}
}

func TestDPT2InvalidInput(t *testing.T) {
	d := DPT_2001(3)
	for _, input := range []string{"", "-1", "4", "1.5", "NaN", "Inf"} {
		_, err := d.ToByteArray(input)
		require.Error(t, err, "input %q", input)
	}
	require.ErrorIs(t, d.Unpack(nil), ErrInvalidLength)
	require.ErrorIs(t, d.Unpack([]byte{0, 0}), ErrInvalidLength)
	require.Equal(t, DPT_2001(3), d)
}
