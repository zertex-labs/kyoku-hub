package deezer

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"

	"golang.org/x/crypto/blowfish"
)

const (
	blowfishSecret = "g4el58wc0zvf9na1"
	chunkSize      = 2048
	aesECBKey      = "jo6aey6haid2Teih"
	cdnSeparator   = "\xa4"
)

var blowfishIV = []byte{0, 1, 2, 3, 4, 5, 6, 7}

func DeriveBlowfishKey(trackID string) []byte {
	hash := md5.Sum([]byte(trackID))
	idMD5 := hex.EncodeToString(hash[:])

	key := make([]byte, 16)
	for i := 0; i < 16; i++ {
		key[i] = idMD5[i] ^ idMD5[i+16] ^ blowfishSecret[i]
	}

	return key
}

func DecryptStream(reader io.Reader, writer io.Writer, trackID string) error {
	key := DeriveBlowfishKey(trackID)

	bf, err := blowfish.NewCipher(key)
	if err != nil {
		return fmt.Errorf("failed to create blowfish cipher: %w", err)
	}

	buf := make([]byte, chunkSize)
	chunkIndex := 0

	for {
		n, readErr := io.ReadFull(reader, buf)
		if n == 0 {
			if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
				break
			}
			if readErr != nil {
				return fmt.Errorf("read error: %w", readErr)
			}
			break
		}

		chunk := buf[:n]

		if chunkIndex%3 == 0 && n == chunkSize {
			iv := make([]byte, 8)
			copy(iv, blowfishIV)
			decrypter := cipher.NewCBCDecrypter(bf, iv)
			decrypter.CryptBlocks(chunk, chunk)
		}

		if _, err := writer.Write(chunk); err != nil {
			return fmt.Errorf("write error: %w", err)
		}

		chunkIndex++

		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break
		}
	}

	return nil
}

func BuildLegacyCDNURL(md5Origin, sngID, mediaVersion, quality string) (string, error) {
	qualityCode := qualityToCode(quality)

	plaintext := md5Origin + cdnSeparator + qualityCode + cdnSeparator + sngID + cdnSeparator + mediaVersion

	hash := md5.Sum([]byte(plaintext))
	hashHex := hex.EncodeToString(hash[:])

	filename := hashHex + cdnSeparator + plaintext + cdnSeparator

	for len(filename)%16 != 0 {
		filename += "."
	}

	encrypted, err := aesECBEncrypt([]byte(aesECBKey), []byte(filename))
	if err != nil {
		return "", fmt.Errorf("AES-ECB encryption failed: %w", err)
	}

	encHex := hex.EncodeToString(encrypted)
	cdnURL := fmt.Sprintf("https://e-cdns-proxy-%c.dzcdn.net/mobile/1/%s", md5Origin[0], encHex)

	return cdnURL, nil
}

func qualityToCode(quality string) string {
	switch quality {
	case "FLAC":
		return "9"
	case "MP3_320":
		return "3"
	case "MP3_128":
		return "1"
	default:
		return "1"
	}
}

func aesECBEncrypt(key, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	if len(data)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("data length %d is not a multiple of AES block size %d", len(data), aes.BlockSize)
	}

	encrypted := make([]byte, len(data))
	for i := 0; i < len(data); i += aes.BlockSize {
		block.Encrypt(encrypted[i:i+aes.BlockSize], data[i:i+aes.BlockSize])
	}

	return encrypted, nil
}
