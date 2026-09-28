package imaging

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestCompressRejectsOversizedHeaderBeforeDecoding(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	for _, dims := range [][2]uint32{{6000, 4001}, {16385, 1}, {1, 16385}} {
		src := append([]byte(nil), buf.Bytes()...)
		binary.BigEndian.PutUint32(src[16:20], dims[0])
		binary.BigEndian.PutUint32(src[20:24], dims[1])
		binary.BigEndian.PutUint32(src[29:33], crc32.ChecksumIEEE(src[12:29]))
		if _, err := CompressJPEG(src, 1200, 85); !errors.Is(err, ErrImageTooLarge) {
			t.Fatalf("dimensions %v: got %v, want header rejection", dims, err)
		}
	}
}

func TestWhiteBackgroundPreservesColors(t *testing.T) {
	// Include a non-zero origin and row padding to exercise image strides.
	src := image.NewNRGBA(image.Rect(0, 0, 6, 3)).SubImage(image.Rect(1, 1, 5, 3)).(*image.NRGBA)
	for y := src.Rect.Min.Y; y < src.Rect.Max.Y; y++ {
		for x := src.Rect.Min.X; x < src.Rect.Max.X; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: 27, G: 113, B: 231, A: uint8((x - 1) * 85)})
		}
	}
	want := image.NewRGBA(src.Bounds())
	draw.Draw(want, want.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(want, want.Bounds(), src, src.Bounds().Min, draw.Over)
	got := onWhite(src)
	for y := src.Rect.Min.Y; y < src.Rect.Max.Y; y++ {
		for x := src.Rect.Min.X; x < src.Rect.Max.X; x++ {
			actual := color.RGBAModel.Convert(got.At(x, y)).(color.RGBA)
			expected := want.RGBAAt(x, y)
			for c, value := range []uint8{actual.R, actual.G, actual.B, actual.A} {
				delta := int(value) - int([]uint8{expected.R, expected.G, expected.B, expected.A}[c])
				if delta < -1 || delta > 1 {
					t.Fatalf("pixel (%d,%d): got %v want %v", x, y, actual, expected)
				}
			}
		}
	}
}

func TestCompressJPEGKeepsEXIFOrientationAndSize(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 80, 40))
	draw.Draw(img, image.Rect(0, 0, 40, 40), image.NewUniform(color.RGBA{R: 255, A: 255}), image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(40, 0, 80, 40), image.NewUniform(color.RGBA{B: 255, A: 255}), image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	// Minimal EXIF TIFF directory: orientation 6 (90 degrees clockwise).
	exif := []byte{'E', 'x', 'i', 'f', 0, 0, 'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0}
	src := append([]byte{0xff, 0xd8, 0xff, 0xe1, 0, byte(len(exif) + 2)}, exif...)
	src = append(src, buf.Bytes()[2:]...)
	compressed, err := CompressJPEG(src, 40, 95)
	if err != nil {
		t.Fatal(err)
	}
	got, err := jpeg.Decode(bytes.NewReader(compressed))
	if err != nil || got.Bounds().Size() != image.Pt(20, 40) {
		t.Fatalf("rotated image: %v, %v", got, err)
	}
	top := color.RGBAModel.Convert(got.At(10, 5)).(color.RGBA)
	bottom := color.RGBAModel.Convert(got.At(10, 35)).(color.RGBA)
	if top.R < 200 || top.B > 30 || bottom.B < 200 || bottom.R > 30 {
		t.Fatalf("orientation changed: top=%v bottom=%v", top, bottom)
	}
}

func benchmarkPhoto(b *testing.B, width, height int, format string) []byte {
	b.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			i := y*img.Stride + x*4
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = byte(x), byte(y), byte(x+y), 255
		}
	}
	var buf bytes.Buffer
	var err error
	if format == "png" {
		err = png.Encode(&buf, img)
	} else {
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	}
	if err != nil {
		b.Fatal(err)
	}
	return buf.Bytes()
}

func BenchmarkCompressJPEG(b *testing.B) {
	for _, format := range []string{"jpeg", "png"} {
		b.Run(format+"_12MP", func(b *testing.B) {
			src := benchmarkPhoto(b, 4000, 3000, format)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := CompressJPEG(src, 1200, 85); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
