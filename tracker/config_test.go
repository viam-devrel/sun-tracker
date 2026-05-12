package tracker

import (
	"testing"

	"go.viam.com/test"
)

func TestApplyDefaults_FillsZerosOnly(t *testing.T) {
	c := &Config{
		VisionService: "v", Camera: "c", PanServo: "p", TiltServo: "t",
	}
	applyDefaults(c)
	test.That(t, c.Kp, test.ShouldEqual, 8.0)
	test.That(t, c.Deadband, test.ShouldEqual, 0.05)
	test.That(t, c.MinBrightness, test.ShouldEqual, 30.0)
	test.That(t, c.PanSign, test.ShouldEqual, 1)
	test.That(t, c.TiltSign, test.ShouldEqual, 1)
	test.That(t, c.PanMax, test.ShouldEqual, uint32(180))
	test.That(t, c.TiltMax, test.ShouldEqual, uint32(180))
	test.That(t, c.LoopHz, test.ShouldEqual, 10.0)
	test.That(t, c.MaxStepDegs, test.ShouldEqual, 5.0)
}

func TestApplyDefaults_PreservesNonZero(t *testing.T) {
	c := &Config{
		VisionService: "v", Camera: "c", PanServo: "p", TiltServo: "t",
		Kp: 3.0, PanSign: -1, LoopHz: 20.0,
	}
	applyDefaults(c)
	test.That(t, c.Kp, test.ShouldEqual, 3.0)
	test.That(t, c.PanSign, test.ShouldEqual, -1)
	test.That(t, c.LoopHz, test.ShouldEqual, 20.0)
}

func TestValidate_RequiresDeps(t *testing.T) {
	c := &Config{}
	_, _, err := c.Validate("services.0")
	test.That(t, err, test.ShouldNotBeNil)

	c = &Config{VisionService: "v", Camera: "c", PanServo: "p", TiltServo: "t"}
	required, _, err := c.Validate("services.0")
	test.That(t, err, test.ShouldBeNil)
	test.That(t, required, test.ShouldResemble, []string{"v", "c", "p", "t"})
}
