// Package covers fetches, re-encodes and stores cover images.
//
// Nothing fetched from outside is stored as it arrived. Every image is
// decoded, bounded, re-encoded and written under a name derived from its own
// content. That single rule removes three problems at once: decompression
// bombs, EXIF metadata nobody asked for, and path traversal through a
// filename taken from a URL.
package covers

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"

	// Registers the decoders. Only these three formats are accepted; anything
	// else fails to decode and is refused.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// Limits on anything decoded from outside.
const (
	// MaxPixels bounds the decoded image. A 100 megapixel PNG compresses to a
	// few kilobytes and expands to hundreds of megabytes in memory, which is
	// how a Raspberry Pi is taken down by a single cover.
	MaxPixels = 40_000_000

	// MaxDimension is the longest side kept. A cover is displayed at a few
	// hundred pixels; storing more fills the volume for nothing.
	MaxDimension = 1000

	// Quality is the JPEG quality of the re-encoded image.
	Quality = 85
)

// ErrNotAnImage reports bytes that are not a usable image.
var ErrNotAnImage = errors.New("not a usable image")

// ErrTooLarge reports an image whose decoded size exceeds MaxPixels.
var ErrTooLarge = errors.New("image is too large to decode safely")

// Normalised is a re-encoded image, ready to store.
type Normalised struct {
	Data   []byte
	Width  int
	Height int
	Mime   string
}

// Normalise decodes raw, refuses anything oversized, downscales it and
// re-encodes it as JPEG.
//
// The dimensions are read from the header before the pixels are decoded, so
// a decompression bomb is refused without ever being expanded.
func Normalise(raw []byte) (Normalised, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return Normalised{}, fmt.Errorf("%w: %v", ErrNotAnImage, err)
	}
	if config.Width <= 0 || config.Height <= 0 {
		return Normalised{}, fmt.Errorf("%w: it claims to be %dx%d",
			ErrNotAnImage, config.Width, config.Height)
	}
	if config.Width*config.Height > MaxPixels {
		return Normalised{}, fmt.Errorf("%w: %dx%d is %d pixels, over the %d limit",
			ErrTooLarge, config.Width, config.Height,
			config.Width*config.Height, MaxPixels)
	}
	_ = format

	decoded, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return Normalised{}, fmt.Errorf("%w: %v", ErrNotAnImage, err)
	}

	decoded = downscale(decoded)

	var out bytes.Buffer
	if err := jpeg.Encode(&out, decoded, &jpeg.Options{Quality: Quality}); err != nil {
		return Normalised{}, fmt.Errorf("re-encoding the cover: %w", err)
	}

	bounds := decoded.Bounds()
	return Normalised{
		Data:   out.Bytes(),
		Width:  bounds.Dx(),
		Height: bounds.Dy(),
		Mime:   "image/jpeg",
	}, nil
}

// downscale shrinks an image whose longest side exceeds MaxDimension, using
// nearest neighbour. A cover thumbnail does not justify a resampling
// dependency, and the result is displayed at a few hundred pixels anyway.
func downscale(src image.Image) image.Image {
	b := src.Bounds()
	longest := b.Dx()
	if b.Dy() > longest {
		longest = b.Dy()
	}
	if longest <= MaxDimension {
		return src
	}

	width := b.Dx() * MaxDimension / longest
	height := b.Dy() * MaxDimension / longest
	dst := image.NewRGBA(image.Rect(0, 0, width, height))

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			srcX := b.Min.X + x*b.Dx()/width
			srcY := b.Min.Y + y*b.Dy()/height
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}
	// Flatten onto white: a transparent PNG would otherwise come out black.
	flat := image.NewRGBA(dst.Bounds())
	draw.Draw(flat, flat.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(flat, flat.Bounds(), dst, image.Point{}, draw.Over)
	return flat
}
