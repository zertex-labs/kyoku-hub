package tagwriter

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"strings"

	flac "github.com/go-flac/go-flac"
)

// Cover embeds the album artwork as a front-cover PICTURE block.
//
// A sidecar alone is not enough: most players read art from the file, so a
// library that only has cover.jpg shows blank artwork in half of them.
type Cover struct {
	// NoEmbed and NoFile are opt-outs, so the zero value keeps the original
	// behaviour: embed into every file and drop cover.jpg beside them.
	NoEmbed bool
	NoFile  bool
}

func (Cover) Name() string { return "cover" }

func (c Cover) Process(ctx context.Context, a *Artifact) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.NoEmbed {
		return Skip("artwork embedding is off in settings")
	}
	if len(a.Cover) == 0 {
		return Skip("the provider had no artwork for this album")
	}
	if !strings.EqualFold(a.Ext, ".flac") {
		return Skip("no picture writer for " + a.Ext + " yet")
	}
	return EmbedFLACCover(a.Path, a.Cover)
}

// EmbedFLACCover writes a front-cover PICTURE block into the file, replacing any
// existing picture.
func EmbedFLACCover(path string, img []byte) error {
	if len(img) == 0 {
		return nil
	}

	mime, w, h := ImageInfo(img)
	if mime == "" {
		return fmt.Errorf("cover: unrecognised image format")
	}

	var buf bytes.Buffer
	u32 := func(v uint32) { _ = binary.Write(&buf, binary.BigEndian, v) }
	u32(3) // picture type 3: front cover
	u32(uint32(len(mime)))
	buf.WriteString(mime)
	u32(0) // empty description
	u32(uint32(w))
	u32(uint32(h))
	u32(24) // colour depth
	u32(0)  // indexed colours (0 = not palette based)
	u32(uint32(len(img)))
	buf.Write(img)

	f, err := flac.ParseFile(path)
	if err != nil {
		return fmt.Errorf("parse flac: %w", err)
	}

	// Drop old pictures first: replacing in place would leave two, and players
	// pick the first one they find.
	meta := make([]*flac.MetaDataBlock, 0, len(f.Meta)+1)
	for _, m := range f.Meta {
		if m.Type == flac.Picture {
			continue
		}
		meta = append(meta, m)
	}
	meta = append(meta, &flac.MetaDataBlock{Type: flac.Picture, Data: buf.Bytes()})
	f.Meta = meta
	return f.Save(path)
}

// ImageInfo returns the MIME type and pixel size of a JPEG or PNG.
func ImageInfo(b []byte) (string, int, int) {
	if len(b) > 24 && b[0] == 0xFF && b[1] == 0xD8 {
		// Walk the JPEG segments looking for a Start Of Frame marker.
		for i := 2; i+9 < len(b); {
			if b[i] != 0xFF {
				i++
				continue
			}
			marker := b[i+1]
			if marker == 0xD8 || marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
				i += 2
				continue
			}
			if i+4 > len(b) {
				break
			}
			segLen := int(b[i+2])<<8 | int(b[i+3])
			if marker >= 0xC0 && marker <= 0xCF && marker != 0xC4 && marker != 0xC8 && marker != 0xCC {
				if i+9 > len(b) {
					break
				}
				h := int(b[i+5])<<8 | int(b[i+6])
				w := int(b[i+7])<<8 | int(b[i+8])
				return "image/jpeg", w, h
			}
			i += 2 + segLen
		}
		return "image/jpeg", 0, 0
	}
	if len(b) > 24 && bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")) {
		w := int(binary.BigEndian.Uint32(b[16:20]))
		h := int(binary.BigEndian.Uint32(b[20:24]))
		return "image/png", w, h
	}
	return "", 0, 0
}
