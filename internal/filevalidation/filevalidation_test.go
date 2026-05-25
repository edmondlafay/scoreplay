package filevalidation

import (
	"io"
	"strings"
	"testing"

	"github.com/edmondlafaydavid/scoreplay/internal/model"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		buf      []byte
		wantType model.MediaType
		wantOk   bool
	}{
		{
			"JPEG",
			[]byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0, 0, 0, 0, 0, 0, 0},
			model.MediaTypePhoto, true,
		},
		{
			"PNG",
			[]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0},
			model.MediaTypePhoto, true,
		},
		{
			"GIF89a",
			[]byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0, 0, 0, 0, 0, 0},
			model.MediaTypePhoto, true,
		},
		{
			"GIF87a",
			[]byte{0x47, 0x49, 0x46, 0x38, 0x37, 0x61, 0, 0, 0, 0, 0, 0},
			model.MediaTypePhoto, true,
		},
		{
			"WebP",
			[]byte{0x52, 0x49, 0x46, 0x46, 0x24, 0, 0, 0, 0x57, 0x45, 0x42, 0x50},
			model.MediaTypePhoto, true,
		},
		{
			"MP4 (isom)",
			[]byte{0, 0, 0, 0x18, 0x66, 0x74, 0x79, 0x70, 0x69, 0x73, 0x6F, 0x6D},
			model.MediaTypeVideo, true,
		},
		{
			"MOV (qt)",
			[]byte{0, 0, 0, 0x14, 0x66, 0x74, 0x79, 0x70, 0x71, 0x74, 0x20, 0x20},
			model.MediaTypeVideo, true,
		},
		{
			"MKV/WebM",
			[]byte{0x1A, 0x45, 0xDF, 0xA3, 0, 0, 0, 0, 0, 0, 0, 0},
			model.MediaTypeVideo, true,
		},
		{
			"AVI",
			[]byte{0x52, 0x49, 0x46, 0x46, 0x10, 0, 0, 0, 0x41, 0x56, 0x49, 0x20},
			model.MediaTypeVideo, true,
		},
		// Rejected formats
		{
			"PDF (rejected)",
			[]byte{0x25, 0x50, 0x44, 0x46, 0, 0, 0, 0, 0, 0, 0, 0},
			"", false,
		},
		{
			"random bytes",
			[]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11},
			"", false,
		},
		// RIFF prefix but not WebP or AVI — rejected
		{
			"RIFF but not WebP/AVI",
			[]byte{0x52, 0x49, 0x46, 0x46, 0, 0, 0, 0, 0x57, 0x41, 0x56, 0x45},
			"", false,
		},
		// Buffer too short for any match
		{
			"too short for JPEG",
			[]byte{0xFF, 0xD8},
			"", false,
		},
		{
			"empty",
			[]byte{},
			"", false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := detect(tc.buf)
			if ok != tc.wantOk {
				t.Fatalf("detect() ok=%v, want %v", ok, tc.wantOk)
			}
			if got != tc.wantType {
				t.Errorf("detect() type=%v, want %v", got, tc.wantType)
			}
		})
	}
}

func TestSniffType_SeeksBack(t *testing.T) {
	// JPEG magic + padding
	content := make([]byte, 64)
	content[0], content[1], content[2] = 0xFF, 0xD8, 0xFF

	r := strings.NewReader(string(content))

	mt, err := SniffType(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mt != model.MediaTypePhoto {
		t.Errorf("expected photo, got %v", mt)
	}

	// Verify the reader was seeked back to 0.
	remaining, _ := io.ReadAll(r)
	if len(remaining) != len(content) {
		t.Errorf("seek didn't reset: got %d bytes remaining, want %d", len(remaining), len(content))
	}
}

func TestSniffType_Unsupported(t *testing.T) {
	r := strings.NewReader("not a media file at all")
	_, err := SniffType(r)
	if err != ErrUnsupportedFormat {
		t.Errorf("want ErrUnsupportedFormat, got %v", err)
	}
}

func TestMaxSize(t *testing.T) {
	if MaxSize(model.MediaTypePhoto) != MaxPhotoSize {
		t.Errorf("photo size wrong")
	}
	if MaxSize(model.MediaTypeVideo) != MaxVideoSize {
		t.Errorf("video size wrong")
	}
}
