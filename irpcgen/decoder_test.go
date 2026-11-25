package irpcgen

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"reflect"
	"runtime"
	"testing"
)

// A length announced by the peer and not followed by data is an error: it
// neither panics nor allocates what it announces.
func TestDecodeHostileLength(t *testing.T) {
	decoders := map[string]struct {
		nilFlag bool
		dec     func(*Decoder) error
	}{
		"string":    {false, func(d *Decoder) error { var v string; return DecString(d, &v) }},
		"byteSlice": {true, func(d *Decoder) error { var v []byte; return DecByteSlice(d, &v) }},
		"boolSlice": {true, func(d *Decoder) error { var v []bool; return DecBoolSlice(d, &v) }},
		"slice": {true, func(d *Decoder) error {
			var v []uint64
			return DecSlice(d, &v, "uint64", DecUint64)
		}},
		"map": {true, func(d *Decoder) error {
			var v map[uint64]uint64
			return DecMap(d, &v, "uint64", DecUint64, "uint64", DecUint64)
		}},
	}
	for name, tc := range decoders {
		for _, l := range []uint64{1 << 30, math.MaxInt64, math.MaxInt64 + 1, math.MaxUint64} {
			var msg []byte
			if tc.nilFlag {
				msg = append(msg, 1)
			}
			msg = binary.AppendUvarint(msg, l)

			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			err := tc.dec(NewDecoder(bytes.NewReader(msg)))
			runtime.ReadMemStats(&after)

			if err == nil {
				t.Errorf("%s of length %d: no error", name, l)
			}
			if got := after.TotalAlloc - before.TotalAlloc; got > 1<<20 {
				t.Errorf("%s of length %d: allocated %d bytes", name, l, got)
			}
		}
	}
}

// Values bigger than what is allocated up front still round-trip.
func TestDecodeLongerThanPrealloc(t *testing.T) {
	for _, n := range []int{0, maxPreallocBytes - 1, maxPreallocBytes, maxPreallocBytes + 1, 5*maxPreallocBytes + 3} {
		bs := make([]byte, n)
		bools := make([]bool, n)
		ints := make([]int, n)
		m := make(map[int]int, n)
		for i := range n {
			bs[i], bools[i], ints[i], m[i] = byte(i), i%3 == 0, i, -i
		}

		var buf bytes.Buffer
		enc := NewEncoder(&buf)
		for _, err := range []error{
			EncByteSlice(enc, bs),
			EncString(enc, string(bs)),
			EncBoolSlice(enc, bools),
			EncSlice(enc, ints, "int", EncInt),
			EncMap(enc, m, "int", EncInt, "int", EncInt),
			enc.Flush(),
		} {
			if err != nil {
				t.Fatal(err)
			}
		}

		var gotBs []byte
		var gotStr string
		var gotBools []bool
		var gotInts []int
		var gotM map[int]int
		dec := NewDecoder(&buf)
		for _, err := range []error{
			DecByteSlice(dec, &gotBs),
			DecString(dec, &gotStr),
			DecBoolSlice(dec, &gotBools),
			DecSlice(dec, &gotInts, "int", DecInt),
			DecMap(dec, &gotM, "int", DecInt, "int", DecInt),
		} {
			if err != nil {
				t.Fatalf("n=%d: %v", n, err)
			}
		}
		if !bytes.Equal(gotBs, bs) || gotStr != string(bs) || !reflect.DeepEqual(gotBools, bools) ||
			!reflect.DeepEqual(gotInts, ints) || !reflect.DeepEqual(gotM, m) {
			t.Errorf("n=%d: round trip mismatch", n)
		}
	}
}

func TestDecodeTruncatedBytes(t *testing.T) {
	msg := binary.AppendUvarint([]byte{1}, 3*maxPreallocBytes)
	msg = append(msg, make([]byte, maxPreallocBytes)...)

	var v []byte
	if err := DecByteSlice(NewDecoder(bytes.NewReader(msg)), &v); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("got %v, want io.ErrUnexpectedEOF", err)
	}
}
