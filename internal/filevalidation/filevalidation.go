package filevalidation

import (
	"fmt"
	"io"

	"github.com/edmondlafaydavid/scoreplay/internal/model"
)

const (
	// MaxPhotoSize is the maximum accepted size for photo uploads.
	MaxPhotoSize int64 = 20 << 20 // 20 MB

	// MaxVideoSize is the maximum accepted size for video uploads.
	MaxVideoSize int64 = 500 << 20 // 500 MB

	sniffLen = 12 // bytes needed to identify all supported formats
)

// MaxSize returns the size limit for a given media type.
func MaxSize(t model.MediaType) int64 {
	if t == model.MediaTypeVideo {
		return MaxVideoSize
	}
	return MaxPhotoSize
}

// SniffType reads the first sniffLen bytes from r to detect the media type
// from magic bytes, then seeks back to the start of the stream.
// Returns ErrUnsupportedFormat if no known signature is found.
func SniffType(r io.ReadSeeker) (model.MediaType, error) {
	buf := make([]byte, sniffLen)
	n, _ := io.ReadFull(r, buf)
	buf = buf[:n]

	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("seek after sniff: %w", err)
	}

	mt, ok := detect(buf)
	if !ok {
		return "", ErrUnsupportedFormat
	}
	return mt, nil
}

// ErrUnsupportedFormat is returned when the file's magic bytes do not match
// any supported photo or video format.
var ErrUnsupportedFormat = fmt.Errorf("unsupported file format")

// detect inspects raw magic bytes and returns the media type.
// Exported as a package-level variable so tests can compare errors directly;
// unexported so callers use SniffType.
func detect(buf []byte) (model.MediaType, bool) {
	switch {
	// ── Photos ────────────────────────────────────────────────────────────────
	// JPEG: FF D8 FF
	case magic(buf, 0, 0xFF, 0xD8, 0xFF):
		return model.MediaTypePhoto, true

	// PNG: 89 50 4E 47 0D 0A 1A 0A
	case magic(buf, 0, 0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A):
		return model.MediaTypePhoto, true

	// GIF87a / GIF89a: 47 49 46 38
	case magic(buf, 0, 0x47, 0x49, 0x46, 0x38):
		return model.MediaTypePhoto, true

	// WebP: RIFF????WEBP
	case magic(buf, 0, 0x52, 0x49, 0x46, 0x46) &&
		magic(buf, 8, 0x57, 0x45, 0x42, 0x50):
		return model.MediaTypePhoto, true

	// ── Videos ────────────────────────────────────────────────────────────────
	// MP4 / MOV: ftyp box at offset 4 (covers mp4, m4v, m4a, qt, etc.)
	case magic(buf, 4, 0x66, 0x74, 0x79, 0x70):
		return model.MediaTypeVideo, true

	// MKV / WebM: EBML header 1A 45 DF A3
	case magic(buf, 0, 0x1A, 0x45, 0xDF, 0xA3):
		return model.MediaTypeVideo, true

	// AVI: RIFF????AVI (space)
	case magic(buf, 0, 0x52, 0x49, 0x46, 0x46) &&
		magic(buf, 8, 0x41, 0x56, 0x49, 0x20):
		return model.MediaTypeVideo, true
	}

	return "", false
}

// magic reports whether buf starting at offset matches all given bytes.
func magic(buf []byte, offset int, bb ...byte) bool {
	if len(buf) < offset+len(bb) {
		return false
	}
	for i, b := range bb {
		if buf[offset+i] != b {
			return false
		}
	}
	return true
}
