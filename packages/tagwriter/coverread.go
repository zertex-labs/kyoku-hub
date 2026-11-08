package tagwriter

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	flac "github.com/go-flac/go-flac"
)

// ErrNoCover means the file carries no picture block, which is normal and not a
// failure.
var ErrNoCover = errors.New("no cover")

// ReadFLACCover returns the front-cover image embedded in a file.
//
// Re-tagging has to reuse the artwork that is already there: the album was
// downloaded at some point, its art came with it, and asking a provider again
// would mean a network round-trip for a picture sitting in the file.
func ReadFLACCover(path string) (cover []byte, err error) {
	// The FLAC parser panics on a file that is truncated or not really FLAC. A
	// corrupt file in the library must not take the whole re-tag down with it.
	defer func() {
		if r := recover(); r != nil {
			cover, err = nil, fmt.Errorf("unreadable flac: %v", r)
		}
	}()

	f, err := flac.ParseFile(path)
	if err != nil {
		return nil, fmt.Errorf("parse flac: %w", err)
	}

	var front []byte
	for _, m := range f.Meta {
		if m.Type != flac.Picture || len(m.Data) < 32 {
			continue
		}
		img, picType, err := decodePicture(m.Data)
		if err != nil {
			continue
		}
		if picType == 3 {
			return img, nil
		}
		if front == nil {
			front = img
		}
	}
	if front == nil {
		return nil, ErrNoCover
	}
	return front, nil
}

// decodePicture reads a FLAC PICTURE block:
// type, mime length, mime, description length, description, width, height,
// depth, colours, data length, data.
func decodePicture(data []byte) ([]byte, uint32, error) {
	be := func(off int) uint32 {
		if off+4 > len(data) {
			return 0
		}
		return binary.BigEndian.Uint32(data[off : off+4])
	}

	picType := be(0)
	pos := 4
	mimeLen := int(be(pos))
	pos += 4 + mimeLen
	if pos+4 > len(data) {
		return nil, 0, ErrNoCover
	}
	descLen := int(be(pos))
	pos += 4 + descLen
	// width, height, depth, colours
	pos += 16
	if pos+4 > len(data) {
		return nil, 0, ErrNoCover
	}
	size := int(be(pos))
	pos += 4
	if size <= 0 || pos+size > len(data) {
		return nil, 0, ErrNoCover
	}
	return bytes.Clone(data[pos : pos+size]), picType, nil
}
