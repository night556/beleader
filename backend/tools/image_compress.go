package tools

import (
	"bytes"
	"image"
	"image/jpeg"
	_ "image/gif"
	_ "image/png"

	"golang.org/x/image/draw"
)

func compressImage(data []byte, maxDim int, quality int) []byte {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data
	}

	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	if w > maxDim || h > maxDim {
		var nw, nh int
		if w >= h {
			nw = maxDim
			nh = h * maxDim / w
		} else {
			nh = maxDim
			nw = w * maxDim / h
		}
		dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
		draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)
		img = dst
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return data
	}
	return buf.Bytes()
}
