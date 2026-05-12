package sunposition

import "image"

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
