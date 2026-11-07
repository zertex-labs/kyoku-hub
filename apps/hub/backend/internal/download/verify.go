package download

import (
	"fmt"
	"os"
	"strings"

	flac "github.com/go-flac/go-flac"
)

// MinTrackBytes is the stub threshold. A CD-quality FLAC track is tens of MB;
// anything under 100 KB is a truncated transfer, an error page saved as audio,
// or a non-audio file wearing a musical extension.
const MinTrackBytes = 100 * 1024

// VerifyAudio is the tier-1 integrity check: cheap, universal, no external
// binary. "The HTTP request returned 200" is not verification — a decrypted
// stream can be half a file and still be 200.
//
// Tier 2 (full decode, checking the STREAMINFO MD5) is deliberately not here:
// it costs a full decode per file, so it belongs behind a setting, not on the
// default path.
func VerifyAudio(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat: %w", err)
	}
	if st.Size() < MinTrackBytes {
		return fmt.Errorf("file too small (%d bytes)", st.Size())
	}

	switch strings.ToLower(ext(path)) {
	case ".flac":
		f, err := flac.ParseFile(path)
		if err != nil {
			return fmt.Errorf("not a valid flac: %w", err)
		}
		if len(f.Meta) == 0 {
			return fmt.Errorf("flac has no metadata blocks")
		}
	case ".mp3":
		if err := checkMP3Frame(path); err != nil {
			return err
		}
	}
	return nil
}

func ext(path string) string {
	i := strings.LastIndexByte(path, '.')
	if i < 0 {
		return ""
	}
	return path[i:]
}

// checkMP3Frame accepts an ID3v2 tag or a bare MPEG frame sync. A file that
// starts with neither is not audio, whatever it is called.
func checkMP3Frame(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	head := make([]byte, 3)
	if _, err := f.Read(head); err != nil {
		return fmt.Errorf("read header: %w", err)
	}
	if string(head) == "ID3" {
		return nil
	}
	if head[0] == 0xFF && head[1]&0xE0 == 0xE0 {
		return nil
	}
	return fmt.Errorf("no mp3 frame sync or id3 tag")
}
