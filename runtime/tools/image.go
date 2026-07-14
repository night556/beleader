package tools

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/gif"
	_ "image/png"
	"strings"

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

func encodeBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func decodeImageConfig(data []byte) (struct{ Width, Height int }, string, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return struct{ Width, Height int }{}, "", err
	}
	return struct{ Width, Height int }{cfg.Width, cfg.Height}, "", nil
}

var imageExts = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".bmp":  "image/bmp",
}

func detectImageMIME(data []byte, ext string) string {
	if len(data) < 4 {
		return ""
	}
	lower := strings.ToLower(ext)
	switch {
	case data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47:
		return "image/png"
	case data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return "image/jpeg"
	case data[0] == 'G' && data[1] == 'I' && data[2] == 'F' && data[3] == '8':
		return "image/gif"
	case data[0] == 'B' && data[1] == 'M':
		return "image/bmp"
	case data[0] == 'R' && data[1] == 'I' && data[2] == 'F' && data[3] == 'F' && len(data) > 11 &&
		data[8] == 'W' && data[9] == 'E' && data[10] == 'B' && data[11] == 'P':
		return "image/webp"
	}
	if mime, ok := imageExts[lower]; ok {
		return mime
	}
	return ""
}

func formatSizeForImage(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(n)/1024)
	}
	return fmt.Sprintf("%.1fMB", float64(n)/(1024*1024))
}
