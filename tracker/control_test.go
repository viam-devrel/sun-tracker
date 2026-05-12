package tracker

import (
	"image"
	"math"
	"testing"

	objdet "go.viam.com/rdk/vision/objectdetection"
	"go.viam.com/test"
)

func TestControl_DeadbandReturnsZero(t *testing.T) {
	step := control(0.03, 0.0, 0.1, 8.0, 0.0, 0.05, 5.0)
	test.That(t, step, test.ShouldEqual, 0.0)
}

func TestControl_OutsideDeadbandReturnsClampedStep(t *testing.T) {
	step := control(0.5, 0.0, 0.1, 8.0, 0.0, 0.05, 5.0)
	// Raw = 8 * 0.5 = 4.0 — under the clamp.
	test.That(t, step, test.ShouldAlmostEqual, 4.0, 1e-9)
}

func TestControl_ClampedToMaxStep(t *testing.T) {
	step := control(1.0, 0.0, 0.1, 20.0, 0.0, 0.05, 5.0)
	// Raw = 20.0 — clipped to 5.0.
	test.That(t, step, test.ShouldEqual, 5.0)
}

func TestControl_NegativeError(t *testing.T) {
	step := control(-0.8, 0.0, 0.1, 8.0, 0.0, 0.05, 5.0)
	test.That(t, step, test.ShouldEqual, -5.0) // clamped on the negative side too
}

func TestControl_DtZeroDoesNotPanic(t *testing.T) {
	step := control(0.5, 0.0, 0.0, 8.0, 2.0, 0.05, 5.0)
	test.That(t, math.IsNaN(step), test.ShouldBeFalse)
	// dt=0 → derivative term skipped → proportional only.
	test.That(t, step, test.ShouldAlmostEqual, 4.0, 1e-9)
}

func TestClamp(t *testing.T) {
	test.That(t, clamp(0.0, -1.0, 1.0), test.ShouldEqual, 0.0)
	test.That(t, clamp(-2.0, -1.0, 1.0), test.ShouldEqual, -1.0)
	test.That(t, clamp(2.0, -1.0, 1.0), test.ShouldEqual, 1.0)
}

func TestStepClamp(t *testing.T) {
	test.That(t, stepClamp(90, 5.0, 0, 180), test.ShouldEqual, uint32(95))
	test.That(t, stepClamp(90, -5.4, 0, 180), test.ShouldEqual, uint32(85))
	test.That(t, stepClamp(178, 5.0, 0, 180), test.ShouldEqual, uint32(180))
	test.That(t, stepClamp(2, -5.0, 0, 180), test.ShouldEqual, uint32(0))
}

func mkDet(label string, score float64) objdet.Detection {
	bounds := image.Rect(0, 0, 100, 100)
	return objdet.NewDetection(bounds, image.Rect(0, 0, 50, 50), score, label)
}

func TestDetectionsToErrors_BrightInTR(t *testing.T) {
	dets := []objdet.Detection{
		mkDet("top-left", 0.10),
		mkDet("top-right", 0.80),
		mkDet("bottom-left", 0.10),
		mkDet("bottom-right", 0.10),
	}
	panErr, tiltErr, brightness, ok := detectionsToErrors(dets)
	test.That(t, ok, test.ShouldBeTrue)
	// total = 1.10. panErr = (0.8+0.1 - 0.1-0.1)/1.10 = 0.7/1.10 ≈ 0.636
	test.That(t, panErr, test.ShouldBeGreaterThan, 0.5)
	test.That(t, tiltErr, test.ShouldBeGreaterThan, 0.5)
	// brightness = mean * 255 = (1.10/4)*255 ≈ 70
	test.That(t, brightness, test.ShouldAlmostEqual, 70.125, 0.1)
}

func TestDetectionsToErrors_WrongCountReturnsNotOK(t *testing.T) {
	_, _, _, ok := detectionsToErrors(nil)
	test.That(t, ok, test.ShouldBeFalse)
	_, _, _, ok = detectionsToErrors([]objdet.Detection{mkDet("top-left", 0.5)})
	test.That(t, ok, test.ShouldBeFalse)
}

func TestDetectionsToErrors_UnknownLabelReturnsNotOK(t *testing.T) {
	dets := []objdet.Detection{
		mkDet("top-left", 0.10),
		mkDet("top-right", 0.80),
		mkDet("xyz", 0.10), // bad label
		mkDet("bottom-right", 0.10),
	}
	_, _, _, ok := detectionsToErrors(dets)
	test.That(t, ok, test.ShouldBeFalse)
}

func TestDetectionsToErrors_AllZerosReturnsZeroErr(t *testing.T) {
	dets := []objdet.Detection{
		mkDet("top-left", 0.0),
		mkDet("top-right", 0.0),
		mkDet("bottom-left", 0.0),
		mkDet("bottom-right", 0.0),
	}
	panErr, tiltErr, brightness, ok := detectionsToErrors(dets)
	test.That(t, ok, test.ShouldBeTrue)
	test.That(t, panErr, test.ShouldEqual, 0.0)
	test.That(t, tiltErr, test.ShouldEqual, 0.0)
	test.That(t, brightness, test.ShouldEqual, 0.0)
}
