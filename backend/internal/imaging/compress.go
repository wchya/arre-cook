package imaging

import (
	"bytes"
	"image"
	"image/draw"
	"image/jpeg"

	imgproc "github.com/disintegration/imaging"
	_ "golang.org/x/image/webp"
)

// CompressJPEG EXIF 方向修正 → 等比缩放到不超过 maxDim → 透明区域铺白底 → 编码为 JPG。
// 手机截图/照片通常能减少 70-90% 体积。
func CompressJPEG(src []byte, maxDim, quality int) ([]byte, error) {
	img, err := imgproc.Decode(bytes.NewReader(src), imgproc.AutoOrientation(true))
	if err != nil {
		return nil, err
	}

	b := img.Bounds()
	if b.Dx() > maxDim || b.Dy() > maxDim {
		img = imgproc.Fit(img, maxDim, maxDim, imgproc.Lanczos)
		b = img.Bounds()
	}

	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Over)

	var out bytes.Buffer
	if err := jpeg.Encode(&out, rgba, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
