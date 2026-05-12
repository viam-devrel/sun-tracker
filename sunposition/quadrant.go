package sunposition

import (
	"image"

	objdet "go.viam.com/rdk/vision/objectdetection"
)

// quadrantGeneric returns mean-luma per quadrant (TL, TR, BL, BR) and overall
// mean luma, each normalized to [0, 1] (255 → 1.0). Slow path: uses At().RGBA()
// per pixel — used when the source image isn't *image.YCbCr or *image.Gray.
func quadrantGeneric(img image.Image) (tl, tr, bl, br, brightness float64) {
	b := img.Bounds()
	midX := (b.Min.X + b.Max.X) / 2
	midY := (b.Min.Y + b.Max.Y) / 2

	var sums [4]float64 // 0=TL, 1=TR, 2=BL, 3=BR
	var counts [4]int

	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bv, _ := img.At(x, y).RGBA()
			// RGBA returns 16-bit values; shift to 8-bit then weight.
			luma := 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(bv>>8)
			i := 0
			if y >= midY {
				i += 2
			}
			if x >= midX {
				i += 1
			}
			sums[i] += luma
			counts[i]++
		}
	}
	norm := func(s float64, n int) float64 {
		if n == 0 {
			return 0
		}
		return (s / float64(n)) / 255.0
	}
	tl = norm(sums[0], counts[0])
	tr = norm(sums[1], counts[1])
	bl = norm(sums[2], counts[2])
	br = norm(sums[3], counts[3])
	total := sums[0] + sums[1] + sums[2] + sums[3]
	totalCount := counts[0] + counts[1] + counts[2] + counts[3]
	if totalCount > 0 {
		brightness = (total / float64(totalCount)) / 255.0
	}
	return
}

// quadrantYCbCr is the fast path. Indexes the Y plane directly — no interface
// dispatch, no allocation, no per-pixel matrix math. Subtracts Rect.Min so
// SubImage views work correctly.
func quadrantYCbCr(img *image.YCbCr) (tl, tr, bl, br, brightness float64) {
	b := img.Rect
	midX := (b.Min.X + b.Max.X) / 2
	midY := (b.Min.Y + b.Max.Y) / 2

	var sums [4]uint64
	var counts [4]int

	for y := b.Min.Y; y < b.Max.Y; y++ {
		rowStart := (y - b.Min.Y) * img.YStride
		for x := b.Min.X; x < b.Max.X; x++ {
			luma := uint64(img.Y[rowStart+(x-b.Min.X)])
			i := 0
			if y >= midY {
				i += 2
			}
			if x >= midX {
				i += 1
			}
			sums[i] += luma
			counts[i]++
		}
	}
	norm := func(s uint64, n int) float64 {
		if n == 0 {
			return 0
		}
		return (float64(s) / float64(n)) / 255.0
	}
	tl = norm(sums[0], counts[0])
	tr = norm(sums[1], counts[1])
	bl = norm(sums[2], counts[2])
	br = norm(sums[3], counts[3])
	total := sums[0] + sums[1] + sums[2] + sums[3]
	totalCount := counts[0] + counts[1] + counts[2] + counts[3]
	if totalCount > 0 {
		brightness = (float64(total) / float64(totalCount)) / 255.0
	}
	return
}

// quadrantError dispatches to a type-specific implementation. Returns mean-luma
// per quadrant (TL, TR, BL, BR) and overall mean — all normalized to [0,1].
func quadrantError(img image.Image) (tl, tr, bl, br, brightness float64) {
	switch t := img.(type) {
	case *image.YCbCr:
		return quadrantYCbCr(t)
	default:
		return quadrantGeneric(img)
	}
}

// buildDetections returns four detections in fixed order TL, TR, BL, BR with
// stable labels. Quadrant rectangles split the bounds at the midpoint.
func buildDetections(bounds image.Rectangle, tl, tr, bl, br float64) []objdet.Detection {
	midX := (bounds.Min.X + bounds.Max.X) / 2
	midY := (bounds.Min.Y + bounds.Max.Y) / 2

	quads := []struct {
		label string
		rect  image.Rectangle
		score float64
	}{
		{"top-left", image.Rect(bounds.Min.X, bounds.Min.Y, midX, midY), tl},
		{"top-right", image.Rect(midX, bounds.Min.Y, bounds.Max.X, midY), tr},
		{"bottom-left", image.Rect(bounds.Min.X, midY, midX, bounds.Max.Y), bl},
		{"bottom-right", image.Rect(midX, midY, bounds.Max.X, bounds.Max.Y), br},
	}
	out := make([]objdet.Detection, 0, 4)
	for _, q := range quads {
		rect := q.rect
		out = append(out, objdet.NewDetection(bounds, rect, q.score, q.label))
	}
	return out
}
