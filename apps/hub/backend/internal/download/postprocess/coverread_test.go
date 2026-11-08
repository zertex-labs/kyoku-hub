package postprocess

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// building a PICTURE block the way the tagger writes one
func pictureBlock(picType uint32, mime string, img []byte) []byte {
	var buf bytes.Buffer
	u32 := func(v uint32) { _ = binary.Write(&buf, binary.BigEndian, v) }
	u32(picType)
	u32(uint32(len(mime)))
	buf.WriteString(mime)
	u32(0)
	u32(1)
	u32(1)
	u32(24)
	u32(0)
	u32(uint32(len(img)))
	buf.Write(img)
	return buf.Bytes()
}

func TestDecodePicture(t *testing.T) {
	img := fakeJPEG()
	block := pictureBlock(3, "image/jpeg", img)

	got, picType, err := decodePicture(block)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if picType != 3 {
		t.Errorf("type = %d, want 3", picType)
	}
	if !bytes.Equal(got, img) {
		t.Errorf("image = %d bytes, want %d", len(got), len(img))
	}
}

// A truncated block must not panic or return half an image.
func TestDecodePictureRejectsTruncatedBlocks(t *testing.T) {
	block := pictureBlock(3, "image/jpeg", fakeJPEG())
	for _, n := range []int{1, 6, 20, len(block) - 40} {
		if n <= 0 || n >= len(block) {
			continue
		}
		if _, _, err := decodePicture(block[:n]); err == nil {
			t.Errorf("truncated at %d bytes decoded successfully", n)
		}
	}
}

// Re-tagging an album that kyoku did not download has to reuse the artwork in
// the files: there is no provider URL to fall back on.
func TestReadFLACCoverFindsTheEmbeddedPicture(t *testing.T) {
	img := fakeJPEG()
	path := filepath.Join(t.TempDir(), "01.flac")

	if err := writeTestFLAC(path, pictureBlock(3, "image/jpeg", img)); err != nil {
		t.Fatalf("fixture: %v", err)
	}

	got, err := ReadFLACCover(path)
	if err != nil {
		t.Fatalf("ReadFLACCover: %v", err)
	}
	if !bytes.Equal(got, img) {
		t.Fatalf("cover = %d bytes, want %d", len(got), len(img))
	}
}

func TestReadFLACCoverWithoutAPicture(t *testing.T) {
	path := filepath.Join(t.TempDir(), "01.flac")
	if err := writeTestFLAC(path, nil); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	if _, err := ReadFLACCover(path); err != ErrNoCover {
		t.Fatalf("err = %v, want ErrNoCover", err)
	}
}

// A non-front picture is still better than nothing when a re-tag has no other
// source, but a front cover always wins.
func TestReadFLACCoverPrefersFront(t *testing.T) {
	other := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0x0d, 'I', 'H', 'D', 'R', 0, 0, 0, 1, 0, 0, 0, 1}
	front := fakeJPEG()

	path := filepath.Join(t.TempDir(), "01.flac")
	if err := writeTestFLAC(path, pictureBlock(4, "image/png", other), pictureBlock(3, "image/jpeg", front)); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	got, err := ReadFLACCover(path)
	if err != nil {
		t.Fatalf("ReadFLACCover: %v", err)
	}
	if !bytes.Equal(got, front) {
		t.Error("picked a non-front picture")
	}
}

// writeTestFLAC writes a FLAC the parser accepts: magic, STREAMINFO, the picture
// blocks, then a frame sync and padding.
func writeTestFLAC(path string, pictures ...[]byte) error {
	var buf bytes.Buffer
	buf.WriteString("fLaC")

	// STREAMINFO, 34 bytes; last-metadata-block only when nothing follows.
	last := byte(0x00)
	if len(pictures) == 0 {
		last = 0x80
	}
	buf.WriteByte(last)
	buf.Write([]byte{0x00, 0x00, 0x22})
	buf.Write(make([]byte, 34))

	for i, data := range pictures {
		last := byte(0x00)
		if i == len(pictures)-1 {
			last = 0x80
		}
		buf.WriteByte(last | 0x06) // type 6 = PICTURE
		n := len(data)
		buf.Write([]byte{byte(n >> 16), byte(n >> 8), byte(n)})
		buf.Write(data)
	}

	buf.Write([]byte{0xFF, 0xF8, 0x69, 0x18, 0x00, 0x00})
	buf.Write(make([]byte, 4096))

	return os.WriteFile(path, buf.Bytes(), 0o644)
}
