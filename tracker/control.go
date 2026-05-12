package tracker

import (
	"math"

	objdet "go.viam.com/rdk/vision/objectdetection"
)

// control returns a step in degrees (signed). Returns 0 inside the deadband.
// dt=0 (first call) skips the derivative term.
func control(err, prevErr, dt, kp, kd, deadband, maxStep float64) float64 {
	if math.Abs(err) < deadband {
		return 0
	}
	step := kp * err
	if dt > 0 {
		step += kd * (err - prevErr) / dt
	}
	return clamp(step, -maxStep, maxStep)
}

func clamp(v, lo, hi float64) float64 {
	switch {
	case v < lo:
		return lo
	case v > hi:
		return hi
	default:
		return v
	}
}

// stepClamp applies a signed angular delta to the current angle, then clamps
// to the [lo, hi] range. The delta is rounded to the nearest integer degree
// since servo positions are uint32.
func stepClamp(current uint32, delta float64, lo, hi uint32) uint32 {
	target := int(current) + int(math.Round(delta))
	if target < int(lo) {
		target = int(lo)
	}
	if target > int(hi) {
		target = int(hi)
	}
	return uint32(target)
}

// detectionsToErrors converts the 4-detection vision output into pan/tilt
// imbalance fractions and a 0-255 brightness proxy. Returns ok=false if the
// detection set is malformed (wrong count or unrecognized label).
func detectionsToErrors(dets []objdet.Detection) (panErr, tiltErr, brightness float64, ok bool) {
	if len(dets) != 4 {
		return 0, 0, 0, false
	}
	var tl, tr, bl, br float64
	var seen [4]bool
	for _, d := range dets {
		switch d.Label() {
		case "top-left":
			tl = d.Score()
			seen[0] = true
		case "top-right":
			tr = d.Score()
			seen[1] = true
		case "bottom-left":
			bl = d.Score()
			seen[2] = true
		case "bottom-right":
			br = d.Score()
			seen[3] = true
		default:
			return 0, 0, 0, false
		}
	}
	for _, s := range seen {
		if !s {
			return 0, 0, 0, false
		}
	}
	total := tl + tr + bl + br
	brightness = (total / 4.0) * 255.0
	if total == 0 {
		return 0, 0, brightness, true
	}
	panErr = (tr + br - tl - bl) / total
	tiltErr = (tl + tr - bl - br) / total
	return panErr, tiltErr, brightness, true
}
