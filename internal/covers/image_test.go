package covers_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/aydam2728/bookplate/internal/covers"
)

// pngOf encodes a real image of the given size.
func pngOf(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encoding the fixture: %v", err)
	}
	return buf.Bytes()
}

// bombHeader builds a valid PNG header that claims enormous dimensions and
// carries no pixel data. This is what a decompression bomb looks like to the
// header parser: a few dozen bytes that promise gigabytes.
func bombHeader(width, height uint32) []byte {
	var out bytes.Buffer
	out.Write([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a})

	var ihdr bytes.Buffer
	ihdr.WriteString("IHDR")
	_ = binary.Write(&ihdr, binary.BigEndian, width)
	_ = binary.Write(&ihdr, binary.BigEndian, height)
	ihdr.Write([]byte{8, 6, 0, 0, 0}) // 8-bit RGBA, no interlace

	_ = binary.Write(&out, binary.BigEndian, uint32(13))
	out.Write(ihdr.Bytes())
	_ = binary.Write(&out, binary.BigEndian, crc32.ChecksumIEEE(ihdr.Bytes()))
	return out.Bytes()
}

func TestNormalise_ReEncodesToJPEG(t *testing.T) {
	got, err := covers.Normalise(pngOf(t, 64, 96))
	if err != nil {
		t.Fatalf("Normalise: %v", err)
	}
	if got.Mime != "image/jpeg" {
		t.Errorf("Mime = %q, want image/jpeg", got.Mime)
	}
	if got.Width != 64 || got.Height != 96 {
		t.Errorf("size = %dx%d, want 64x96", got.Width, got.Height)
	}
	if len(got.Data) < 2 || got.Data[0] != 0xff || got.Data[1] != 0xd8 {
		t.Error("the output does not start with a JPEG marker, so it was not re-encoded")
	}
}

func TestNormalise_RefusesADecompressionBomb(t *testing.T) {
	// 60000 x 60000 is 3.6 billion pixels. Decoding it would exhaust the
	// memory of the reference machine long before the image appeared.
	_, err := covers.Normalise(bombHeader(60000, 60000))
	if !errors.Is(err, covers.ErrTooLarge) {
		t.Fatalf("a 3.6 gigapixel image was accepted, err = %v", err)
	}
}

func TestNormalise_RefusesAZeroSizedImage(t *testing.T) {
	_, err := covers.Normalise(bombHeader(0, 0))
	if err == nil {
		t.Error("an image claiming zero dimensions was accepted")
	}
}

func TestNormalise_RefusesBytesThatAreNotAnImage(t *testing.T) {
	for _, bad := range [][]byte{
		[]byte("<html>not found</html>"),
		{},
		[]byte{0x89, 'P', 'N', 'G'},
	} {
		if _, err := covers.Normalise(bad); !errors.Is(err, covers.ErrNotAnImage) {
			t.Errorf("%q was accepted as an image, err = %v", bad, err)
		}
	}
}

func TestNormalise_DownscalesAnOversizedCover(t *testing.T) {
	got, err := covers.Normalise(pngOf(t, 2400, 1600))
	if err != nil {
		t.Fatalf("Normalise: %v", err)
	}
	if got.Width > covers.MaxDimension || got.Height > covers.MaxDimension {
		t.Errorf("size = %dx%d, want neither side over %d",
			got.Width, got.Height, covers.MaxDimension)
	}
	// The aspect ratio must survive, or every cover looks squashed.
	ratio := float64(got.Width) / float64(got.Height)
	if ratio < 1.4 || ratio > 1.6 {
		t.Errorf("aspect ratio %.2f, want about 1.5", ratio)
	}
}

func TestNormalise_LeavesASmallCoverAtItsSize(t *testing.T) {
	got, err := covers.Normalise(pngOf(t, 300, 450))
	if err != nil {
		t.Fatalf("Normalise: %v", err)
	}
	if got.Width != 300 || got.Height != 450 {
		t.Errorf("size = %dx%d, want it untouched at 300x450", got.Width, got.Height)
	}
}
