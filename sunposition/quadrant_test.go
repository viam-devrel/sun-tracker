package sunposition

import (
	"image"
	"image/color"
	"image/draw"
	"testing"

	objdet "go.viam.com/rdk/vision/objectdetection"
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

func TestBuildDetections_FourInFixedOrder(t *testing.T) {
	bounds := image.Rect(0, 0, 200, 100)
	dets := buildDetections(bounds, 0.10, 0.80, 0.30, 0.50)

	test.That(t, len(dets), test.ShouldEqual, 4)

	expected := []struct {
		label string
		score float64
		rect  image.Rectangle
	}{
		{"top-left", 0.10, image.Rect(0, 0, 100, 50)},
		{"top-right", 0.80, image.Rect(100, 0, 200, 50)},
		{"bottom-left", 0.30, image.Rect(0, 50, 100, 100)},
		{"bottom-right", 0.50, image.Rect(100, 50, 200, 100)},
	}
	for i, want := range expected {
		test.That(t, dets[i].Label(), test.ShouldEqual, want.label)
		test.That(t, dets[i].Score(), test.ShouldAlmostEqual, want.score, 1e-9)
		bb := dets[i].BoundingBox()
		test.That(t, *bb, test.ShouldResemble, want.rect)
	}
	_ = objdet.Detection(nil) // silence unused import if dropped
}

// ycbcrFromRGBA converts an RGBA test image to a *image.YCbCr at full chroma
// resolution so test inputs match exactly.
func ycbcrFromRGBA(t *testing.T, src *image.RGBA) *image.YCbCr {
	t.Helper()
	ycbcr := image.NewYCbCr(src.Bounds(), image.YCbCrSubsampleRatio444)
	for y := src.Bounds().Min.Y; y < src.Bounds().Max.Y; y++ {
		for x := src.Bounds().Min.X; x < src.Bounds().Max.X; x++ {
			r, g, b, _ := src.At(x, y).RGBA()
			yy, cb, cr := rgbToYCbCr(uint8(r>>8), uint8(g>>8), uint8(b>>8))
			yi := ycbcr.YOffset(x, y)
			ci := ycbcr.COffset(x, y)
			ycbcr.Y[yi] = yy
			ycbcr.Cb[ci] = cb
			ycbcr.Cr[ci] = cr
		}
	}
	_ = draw.Draw // silence unused if draw becomes unused
	return ycbcr
}

// Same conversion the stdlib uses (image/color/ycbcr.go).
func rgbToYCbCr(r, g, b uint8) (uint8, uint8, uint8) {
	fr := float64(r)
	fg := float64(g)
	fb := float64(b)
	y := 0.299*fr + 0.587*fg + 0.114*fb
	cb := -0.168736*fr - 0.331264*fg + 0.5*fb + 128
	cr := 0.5*fr - 0.418688*fg - 0.081312*fb + 128
	clip := func(v float64) uint8 {
		switch {
		case v < 0:
			return 0
		case v > 255:
			return 255
		default:
			return uint8(v)
		}
	}
	return clip(y), clip(cb), clip(cr)
}

func TestQuadrantYCbCr_ParityWithGeneric(t *testing.T) {
	rgba := newRGBAWithBrightTR()
	ycbcr := ycbcrFromRGBA(t, rgba)

	gtl, gtr, gbl, gbr, gbright := quadrantGeneric(rgba)
	ytl, ytr, ybl, ybr, ybright := quadrantYCbCr(ycbcr)

	// Conversion + integer rounding can introduce small differences. 0.01 = 2.5/255 luma drift max.
	test.That(t, ytl, test.ShouldAlmostEqual, gtl, 0.01)
	test.That(t, ytr, test.ShouldAlmostEqual, gtr, 0.01)
	test.That(t, ybl, test.ShouldAlmostEqual, gbl, 0.01)
	test.That(t, ybr, test.ShouldAlmostEqual, gbr, 0.01)
	test.That(t, ybright, test.ShouldAlmostEqual, gbright, 0.01)
}

func TestQuadrantYCbCr_SubImageView(t *testing.T) {
	full := image.NewYCbCr(image.Rect(0, 0, 200, 200), image.YCbCrSubsampleRatio444)
	for i := range full.Y {
		full.Y[i] = 128
	}
	sub := full.SubImage(image.Rect(100, 100, 200, 200)).(*image.YCbCr)
	tl, tr, bl, br, brightness := quadrantYCbCr(sub)
	test.That(t, tl, test.ShouldAlmostEqual, 128.0/255.0, 0.01)
	test.That(t, tr, test.ShouldAlmostEqual, 128.0/255.0, 0.01)
	test.That(t, bl, test.ShouldAlmostEqual, 128.0/255.0, 0.01)
	test.That(t, br, test.ShouldAlmostEqual, 128.0/255.0, 0.01)
	test.That(t, brightness, test.ShouldAlmostEqual, 128.0/255.0, 0.01)
}
