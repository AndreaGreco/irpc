package irpcgen

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"slices"
)

// Limits on what is allocated on the strength of a length read from the peer,
// before any of the data it announces has been read. They do not limit the size of messages.
const (
	maxPreallocBytes = 64 << 10
	maxPreallocElems = 256
)

// Decoder is a binary decoder that reads various data types from an io.Reader.
//
// It is meant to be used by generated code to decode messages in the IRPC protocol.
type Decoder struct {
	r   *bufio.Reader
	buf []byte
}

func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{
		r:   bufio.NewReader(r),
		buf: make([]byte, binary.MaxVarintLen64),
	}
}

func (d *Decoder) bool() (bool, error) {
	val, err := d.r.ReadByte()
	if err != nil {
		return false, err
	}
	switch val {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, fmt.Errorf("unexpected bool value: %d", val)
	}
}

func (d *Decoder) isNil() (bool, error) {
	isNotNil, err := d.bool()
	if err != nil {
		return false, err
	}

	return !isNotNil, nil
}

func (d *Decoder) byte() (byte, error) {
	return d.r.ReadByte()
}

func (d *Decoder) varInt64InRange(min, max int64) (int64, error) {
	val, err := binary.ReadVarint(d.r)
	if err != nil {
		return 0, err
	}
	if val < min || val > max {
		return 0, fmt.Errorf("varint val %d is outside <%d, %d>", val, min, max)
	}
	return val, nil
}

func (d *Decoder) uvarInt64InRange(max uint64) (uint64, error) {
	val, err := binary.ReadUvarint(d.r)
	if err != nil {
		return 0, err
	}
	if val > max {
		return 0, fmt.Errorf("uvarint val %d is bigger than %d", val, max)
	}
	return val, nil
}

func (d *Decoder) varInt64() (int64, error) {
	return binary.ReadVarint(d.r)
}

func (d *Decoder) uVarInt64() (uint64, error) {
	return binary.ReadUvarint(d.r)
}

func (d *Decoder) len() (int, error) {
	l64, err := d.uVarInt64()
	if err != nil {
		return 0, fmt.Errorf("slice len: %w", err)
	}
	if l64 > math.MaxInt {
		return 0, fmt.Errorf("len %d overflows int", l64)
	}
	return int(l64), nil
}

// bytes reads l bytes. l comes from the peer, so memory grows with the data that
// actually arrives rather than being allocated up front.
func (d *Decoder) bytes(l int) ([]byte, error) {
	s := make([]byte, 0, min(l, maxPreallocBytes))
	for len(s) < l {
		have := len(s)
		n := min(l-have, max(have, maxPreallocBytes))
		s = slices.Grow(s, n)[:have+n]
		if _, err := io.ReadFull(d.r, s[have:]); err != nil {
			if have > 0 && err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return nil, err
		}
	}
	return s, nil
}

func (d *Decoder) float32le() (float32, error) {
	if _, err := io.ReadFull(d.r, d.buf[:4]); err != nil {
		return 0, err
	}
	f := math.Float32frombits(binary.LittleEndian.Uint32(d.buf[:4]))

	return f, nil
}

func (d *Decoder) float64le() (float64, error) {
	ui64, err := d.uint64le()
	if err != nil {
		return 0, err
	}
	f := math.Float64frombits(ui64)
	return f, nil
}

func (d *Decoder) uint64le() (uint64, error) {
	if _, err := io.ReadFull(d.r, d.buf[:8]); err != nil {
		return 0, err
	}
	v := binary.LittleEndian.Uint64(d.buf[:8])
	return v, nil
}

func (d *Decoder) byteSliceNonNil() ([]byte, error) {
	l, err := d.len()
	if err != nil {
		return nil, fmt.Errorf("slice len: %w", err)
	}
	return d.bytes(l)
}

func (d *Decoder) string() (string, error) {
	bs, err := d.byteSliceNonNil()
	if err != nil {
		return "", err
	}
	return string(bs), nil
}
