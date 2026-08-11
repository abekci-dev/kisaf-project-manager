// Package icon draws the kisaf app icon.
//
// The artwork is code rather than a committed .png/.ico for two reasons: the
// repository stays entirely text (readable diffs, no binary blobs), and there is
// no "run the icon generator first" step between `git clone` and `go build`.
// Images are rendered on demand and cached, so the cost is paid once per size
// per process — a few milliseconds — and never again.
package icon

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"sync"
)

// Palette of the mark: a blue tile, a white folder, a commit line.
var (
	tileTop     = color.RGBA{0x62, 0xA0, 0xFF, 0xFF}
	tileBottom  = color.RGBA{0x2F, 0x62, 0xD8, 0xFF}
	folderBack  = color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
	folderFront = color.RGBA{0xE2, 0xEC, 0xFF, 0xFF}
	commitInk   = color.RGBA{0x35, 0x6B, 0xE0, 0xFF}
)

var (
	cacheMu sync.Mutex
	cache   = map[string][]byte{}
)

// PNG returns the icon at the given pixel size, encoded as PNG.
// inset > 0 shrinks the artwork and squares off the tile, which is what a
// maskable PWA icon needs so a launcher can crop it to any shape.
func PNG(size int, inset float64) ([]byte, error) {
	key := fmt.Sprintf("png-%d-%.2f", size, inset)

	cacheMu.Lock()
	if hit, ok := cache[key]; ok {
		cacheMu.Unlock()
		return hit, nil
	}
	cacheMu.Unlock()

	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, render(size, inset)); err != nil {
		return nil, err
	}
	out := buf.Bytes()

	cacheMu.Lock()
	cache[key] = out
	cacheMu.Unlock()
	return out, nil
}

// icoSizes are the resolutions Windows picks between for the notification area,
// the taskbar, and the file properties dialog.
var icoSizes = []int{16, 24, 32, 48, 64, 128, 256}

// ICO returns a multi-resolution Windows icon.
func ICO() ([]byte, error) {
	const key = "ico"

	cacheMu.Lock()
	if hit, ok := cache[key]; ok {
		cacheMu.Unlock()
		return hit, nil
	}
	cacheMu.Unlock()

	type entry struct {
		size         int
		length, offs uint32
	}

	var payload bytes.Buffer
	entries := make([]entry, 0, len(icoSizes))
	headerSize := 6 + 16*len(icoSizes)

	for _, size := range icoSizes {
		var buf bytes.Buffer
		if err := png.Encode(&buf, render(size, 0)); err != nil {
			return nil, err
		}
		entries = append(entries, entry{
			size:   size,
			length: uint32(buf.Len()),
			offs:   uint32(headerSize + payload.Len()),
		})
		payload.Write(buf.Bytes())
	}

	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, uint16(0)) // reserved
	_ = binary.Write(&out, binary.LittleEndian, uint16(1)) // 1 = icon (not cursor)
	_ = binary.Write(&out, binary.LittleEndian, uint16(len(entries)))

	for _, e := range entries {
		// A zero in the dimension byte means 256; that is why it is mod 256.
		out.WriteByte(byte(e.size % 256))
		out.WriteByte(byte(e.size % 256))
		out.WriteByte(0)                                        // palette entries
		out.WriteByte(0)                                        // reserved
		_ = binary.Write(&out, binary.LittleEndian, uint16(1))  // colour planes
		_ = binary.Write(&out, binary.LittleEndian, uint16(32)) // bits per pixel
		_ = binary.Write(&out, binary.LittleEndian, e.length)
		_ = binary.Write(&out, binary.LittleEndian, e.offs)
	}
	out.Write(payload.Bytes())
	result := out.Bytes()

	cacheMu.Lock()
	cache[key] = result
	cacheMu.Unlock()
	return result, nil
}

// ---------------------------------------------------------------------------
// Drawing
// ---------------------------------------------------------------------------

// supersampleFor trades render time against edge quality. Small icons are the
// ones that need the most help, and they are cheap to oversample heavily.
func supersampleFor(size int) int {
	switch {
	case size <= 64:
		return 4
	case size <= 192:
		return 3
	default:
		return 2
	}
}

func render(size int, inset float64) *image.RGBA {
	ss := supersampleFor(size)
	s := size * ss
	canvas := image.NewRGBA(image.Rect(0, 0, s, s))

	radius := float64(s) * 0.22
	if inset > 0 {
		radius = 0 // maskable icons are cropped by the launcher, so bleed to the edge
	}
	drawTile(canvas, radius)
	drawFolder(canvas, inset)

	return downsample(canvas, size, ss)
}

func drawTile(dst *image.RGBA, radius float64) {
	b := dst.Bounds()
	w, h := float64(b.Dx()), float64(b.Dy())

	for y := 0; y < b.Dy(); y++ {
		t := float64(y) / (h - 1)
		c := color.RGBA{
			R: lerp(tileTop.R, tileBottom.R, t),
			G: lerp(tileTop.G, tileBottom.G, t),
			B: lerp(tileTop.B, tileBottom.B, t),
			A: 0xFF,
		}
		for x := 0; x < b.Dx(); x++ {
			if radius > 0 && !insideRounded(float64(x), float64(y), w, h, radius) {
				continue
			}
			dst.SetRGBA(x, y, c)
		}
	}
}

// drawFolder paints a folder with a tab, a front flap, and three linked commit
// dots. It has to stay legible at 16 px — that is where this icon spends most
// of its life, in the notification area.
func drawFolder(dst *image.RGBA, inset float64) {
	s := float64(dst.Bounds().Dx())
	scale := 1.0 - inset*2
	// Maps design space (0..1) onto the canvas.
	px := func(v float64) float64 { return (inset + v*scale) * s }

	fillRounded(dst, px(0.16), px(0.27), px(0.50), px(0.45), px(0.035), folderBack) // tab
	fillRounded(dst, px(0.16), px(0.33), px(0.84), px(0.76), px(0.045), folderBack) // body
	fillRounded(dst, px(0.16), px(0.42), px(0.84), px(0.76), px(0.045), folderFront)

	dotY := px(0.59)
	radius := px(0.036) * 0.55
	fillRect(dst, px(0.34), dotY-px(0.006), px(0.66), dotY+px(0.006), commitInk)
	for _, cx := range []float64{0.34, 0.50, 0.66} {
		fillCircle(dst, px(cx), dotY, radius, commitInk)
	}
}

func fillRect(dst *image.RGBA, x0, y0, x1, y1 float64, c color.RGBA) {
	for y := int(y0); y < int(y1); y++ {
		for x := int(x0); x < int(x1); x++ {
			setOverTile(dst, x, y, c)
		}
	}
}

func fillRounded(dst *image.RGBA, x0, y0, x1, y1, radius float64, c color.RGBA) {
	w, h := x1-x0, y1-y0
	for y := int(y0); y < int(y1); y++ {
		for x := int(x0); x < int(x1); x++ {
			if insideRounded(float64(x)-x0, float64(y)-y0, w, h, radius) {
				setOverTile(dst, x, y, c)
			}
		}
	}
}

func fillCircle(dst *image.RGBA, cx, cy, r float64, c color.RGBA) {
	for y := int(cy - r); y <= int(cy+r); y++ {
		for x := int(cx - r); x <= int(cx+r); x++ {
			dx, dy := float64(x)-cx, float64(y)-cy
			if dx*dx+dy*dy <= r*r {
				setOverTile(dst, x, y, c)
			}
		}
	}
}

// setOverTile refuses to paint on transparent pixels, so the folder can never
// spill outside the rounded tile.
func setOverTile(dst *image.RGBA, x, y int, c color.RGBA) {
	if !(image.Point{X: x, Y: y}).In(dst.Bounds()) {
		return
	}
	if dst.RGBAAt(x, y).A == 0 {
		return
	}
	dst.SetRGBA(x, y, c)
}

func insideRounded(x, y, w, h, r float64) bool {
	if r <= 0 {
		return x >= 0 && y >= 0 && x < w && y < h
	}
	r = math.Min(r, math.Min(w/2, h/2))

	cx, cy := x, y
	switch {
	case x < r:
		cx = r
	case x > w-r:
		cx = w - r
	}
	switch {
	case y < r:
		cy = r
	case y > h-r:
		cy = h - r
	}
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy <= r*r
}

// downsample box-filters the oversampled canvas. This is the only antialiasing
// in the package, and it is why none of the drawing helpers need any.
func downsample(src *image.RGBA, size, ss int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	n := ss * ss

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a int
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					c := src.RGBAAt(x*ss+sx, y*ss+sy)
					// Premultiply, or transparent pixels darken the edges.
					r += int(c.R) * int(c.A) / 255
					g += int(c.G) * int(c.A) / 255
					b += int(c.B) * int(c.A) / 255
					a += int(c.A)
				}
			}
			avgA := a / n
			if avgA == 0 {
				continue
			}
			dst.SetRGBA(x, y, color.RGBA{
				R: clamp(r / n * 255 / avgA),
				G: clamp(g / n * 255 / avgA),
				B: clamp(b / n * 255 / avgA),
				A: uint8(avgA),
			})
		}
	}
	return dst
}

func clamp(v int) uint8 {
	switch {
	case v < 0:
		return 0
	case v > 255:
		return 255
	default:
		return uint8(v)
	}
}

func lerp(a, b uint8, t float64) uint8 {
	return uint8(math.Round(float64(a) + (float64(b)-float64(a))*t))
}
