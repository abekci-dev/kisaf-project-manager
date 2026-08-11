package icon

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"testing"
)

func TestPNGDecodesAtRequestedSize(t *testing.T) {
	for _, size := range []int{32, 192, 512} {
		data, err := PNG(size, 0)
		if err != nil {
			t.Fatalf("%d px: %v", size, err)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%d px did not decode: %v", size, err)
		}
		if b := img.Bounds(); b.Dx() != size || b.Dy() != size {
			t.Errorf("asked for %d px, got %dx%d", size, b.Dx(), b.Dy())
		}
	}
}

// TestPNGIsOpaqueInTheMiddle catches a silent regression in the drawing code:
// an all-transparent image would still decode fine and still be the right size.
func TestPNGIsOpaqueInTheMiddle(t *testing.T) {
	data, err := PNG(64, 0)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, a := img.At(32, 32).RGBA(); a == 0 {
		t.Error("the middle of the icon is transparent — nothing was drawn")
	}
	// The corner must be transparent, that is what rounding produces.
	if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
		t.Error("the corner is not transparent — rounding was not applied")
	}
}

func TestMaskableBleedsToTheEdge(t *testing.T) {
	data, err := PNG(64, 0.14)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	// A maskable icon gets cropped by the platform, so it has to reach the corner.
	if _, _, _, a := img.At(0, 0).RGBA(); a == 0 {
		t.Error("the corner of the maskable icon is transparent")
	}
}

func TestICOHeader(t *testing.T) {
	data, err := ICO()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 22 {
		t.Fatalf("file too short: %d bytes", len(data))
	}
	if binary.LittleEndian.Uint16(data[0:2]) != 0 {
		t.Error("the reserved field is not zero")
	}
	if binary.LittleEndian.Uint16(data[2:4]) != 1 {
		t.Error("the type field must be 1 (icon)")
	}

	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if count != len(icoSizes) {
		t.Fatalf("%d images, wanted %d", count, len(icoSizes))
	}

	// Every entry must point at a PNG that actually decodes.
	for i := 0; i < count; i++ {
		entry := data[6+16*i : 6+16*(i+1)]
		length := binary.LittleEndian.Uint32(entry[8:12])
		offset := binary.LittleEndian.Uint32(entry[12:16])

		if int(offset+length) > len(data) {
			t.Fatalf("entry %d points past the end of the file", i)
		}
		img, err := png.Decode(bytes.NewReader(data[offset : offset+length]))
		if err != nil {
			t.Errorf("entry %d did not decode: %v", i, err)
			continue
		}
		want := icoSizes[i]
		if img.Bounds().Dx() != want {
			t.Errorf("entry %d: %d px, wanted %d", i, img.Bounds().Dx(), want)
		}
		// 256 px is encoded as 0 in the size byte.
		if int(entry[0]) != want%256 {
			t.Errorf("entry %d: size in header is %d", i, entry[0])
		}
	}
}

func TestCacheReturnsSameBytes(t *testing.T) {
	first, err := PNG(48, 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := PNG(48, 0)
	if err != nil {
		t.Fatal(err)
	}
	if &first[0] != &second[0] {
		t.Error("the second call did not come from the cache")
	}
}

func BenchmarkICO(b *testing.B) {
	for i := 0; i < b.N; i++ {
		cacheMu.Lock()
		delete(cache, "ico")
		cacheMu.Unlock()
		if _, err := ICO(); err != nil {
			b.Fatal(err)
		}
	}
}
