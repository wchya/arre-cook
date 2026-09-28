package imaging

import (
	"bytes"
	"errors"
	"image"
	"image/draw"
	"image/jpeg"

	imgproc "github.com/disintegration/imaging"
	_ "golang.org/x/image/webp"
)

// Bound decoded pixels as well as compressed bytes: a small PNG may expand to
// hundreds of MiB. Check the header before the decoder allocates its pixel data.
const MaxInputPixels = 24_000_000
const MaxInputDimension = 16384

var ErrImageTooLarge = errors.New("image dimensions exceed upload limit")

// CompressJPEG EXIF 方向修正 → 等比缩放到不超过 maxDim → 透明区域铺白底 → 编码为 JPG。
// 手机截图/照片通常能减少 70-90% 体积。
func CompressJPEG(src []byte, maxDim, quality int) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > MaxInputDimension || cfg.Height > MaxInputDimension || cfg.Width > MaxInputPixels/cfg.Height {
		return nil, ErrImageTooLarge
	}
	img, err := imgproc.Decode(bytes.NewReader(src), imgproc.AutoOrientation(true))
	if err != nil {
		return nil, err
	}

	b := img.Bounds()
	if b.Dx() > maxDim || b.Dy() > maxDim {
		img = imgproc.Fit(img, maxDim, maxDim, imgproc.Lanczos)
	}

	var out bytes.Buffer
	if err := jpeg.Encode(&out, onWhite(img), &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// The decoded/resized image is private to this request. Reuse its pixel buffer
// instead of allocating another full-size RGBA just to flatten transparency.
func onWhite(img image.Image) image.Image {
	switch img := img.(type) {
	case *image.NRGBA:
		for y := 0; y < img.Rect.Dy(); y++ {
			row := img.Pix[y*img.Stride : y*img.Stride+img.Rect.Dx()*4]
			for i := 0; i < len(row); i += 4 {
				a := uint32(row[i+3])
				if a != 255 {
					for c := 0; c < 3; c++ {
						row[i+c] = uint8((uint32(row[i+c])*a + 255*(255-a) + 127) / 255)
					}
					row[i+3] = 255
				}
			}
		}
		// Opaque NRGBA and RGBA have identical pixels; the JPEG encoder has a
		// specialized RGBA path which also avoids per-pixel interface allocations.
		return &image.RGBA{Pix: img.Pix, Stride: img.Stride, Rect: img.Rect}
	case *image.YCbCr, *image.Gray:
		return img
	case *image.RGBA:
		if img.Opaque() {
			return img
		}
	}
	b := img.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Over)
	return rgba
}
