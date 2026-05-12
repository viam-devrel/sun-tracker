package sunposition

import (
	"image"
	"image/color"
	"testing"

	"go.viam.com/test"
)

// fillRect paints a rectangle on an RGBA image with a flat color.
func fillRect(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func TestQuadrantGeneric_TopRightBright(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	// Fill the top-right quadrant with white; rest is black (zero value).
	fillRect(img, image.Rect(50, 0, 100, 50), color.RGBA{255, 255, 255, 255})

	tl, tr, bl, br, brightness := quadrantGeneric(img)

	test.That(t, tr, test.ShouldBeGreaterThan, tl)
	test.That(t, tr, test.ShouldBeGreaterThan, bl)
	test.That(t, tr, test.ShouldBeGreaterThan, br)
	test.That(t, brightness, test.ShouldBeGreaterThan, 0.0)
}

func TestQuadrantGeneric_AllFourQuadrants(t *testing.T) {
	cases := []struct {
		name   string
		rect   image.Rectangle
		winner string // "tl"|"tr"|"bl"|"br"
	}{
		{"TL", image.Rect(0, 0, 50, 50), "tl"},
		{"TR", image.Rect(50, 0, 100, 50), "tr"},
		{"BL", image.Rect(0, 50, 50, 100), "bl"},
		{"BR", image.Rect(50, 50, 100, 100), "br"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img := image.NewRGBA(image.Rect(0, 0, 100, 100))
			fillRect(img, tc.rect, color.RGBA{255, 255, 255, 255})
			tl, tr, bl, br, _ := quadrantGeneric(img)
			scores := map[string]float64{"tl": tl, "tr": tr, "bl": bl, "br": br}
			winner := scores[tc.winner]
			for k, v := range scores {
				if k == tc.winner {
					continue
				}
				test.That(t, winner, test.ShouldBeGreaterThan, v)
			}
		})
	}
}

func TestQuadrantGeneric_UniformImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	fillRect(img, img.Bounds(), color.RGBA{128, 128, 128, 255})
	tl, tr, bl, br, brightness := quadrantGeneric(img)
	// All four near-equal.
	test.That(t, tl-tr, test.ShouldAlmostEqual, 0.0, 0.01)
	test.That(t, tl-bl, test.ShouldAlmostEqual, 0.0, 0.01)
	test.That(t, tl-br, test.ShouldAlmostEqual, 0.0, 0.01)
	// 128/255 ≈ 0.50.
	test.That(t, brightness, test.ShouldAlmostEqual, 0.5, 0.01)
}
