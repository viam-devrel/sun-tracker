package tracker

import "math"

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
