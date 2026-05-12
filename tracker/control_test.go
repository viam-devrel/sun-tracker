package tracker

import (
	"math"
	"testing"

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
