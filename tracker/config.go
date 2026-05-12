package tracker

import "errors"

type Config struct {
	VisionService string `json:"vision_service"`
	Camera        string `json:"camera"`
	PanServo      string `json:"pan_servo"`
	TiltServo     string `json:"tilt_servo"`

	Kp            float64 `json:"kp,omitempty"`
	Kd            float64 `json:"kd,omitempty"`
	Deadband      float64 `json:"deadband,omitempty"`
	MinBrightness float64 `json:"min_brightness,omitempty"` // 0-255 mean luma

	PanSign       int    `json:"pan_sign,omitempty"`
	TiltSign      int    `json:"tilt_sign,omitempty"`
	PanMin        uint32 `json:"pan_min,omitempty"`
	PanMax        uint32 `json:"pan_max,omitempty"`
	TiltMin       uint32 `json:"tilt_min,omitempty"`
	TiltMax       uint32 `json:"tilt_max,omitempty"`

	LoopHz       float64 `json:"loop_hz,omitempty"`
	MaxStepDegs  float64 `json:"max_step_degs,omitempty"`
}

func (c *Config) Validate(path string) ([]string, []string, error) {
	if c.VisionService == "" || c.Camera == "" || c.PanServo == "" || c.TiltServo == "" {
		return nil, nil, errors.New(path + ": vision_service, camera, pan_servo, tilt_servo are required")
	}
	return []string{c.VisionService, c.Camera, c.PanServo, c.TiltServo}, nil, nil
}

func applyDefaults(c *Config) {
	if c.Kp == 0 {
		c.Kp = 8.0
	}
	if c.Deadband == 0 {
		c.Deadband = 0.05
	}
	if c.MinBrightness == 0 {
		c.MinBrightness = 30.0
	}
	if c.PanSign == 0 {
		c.PanSign = 1
	}
	if c.TiltSign == 0 {
		c.TiltSign = 1
	}
	if c.PanMax == 0 {
		c.PanMax = 180
	}
	if c.TiltMax == 0 {
		c.TiltMax = 180
	}
	if c.LoopHz == 0 {
		c.LoopHz = 10
	}
	if c.MaxStepDegs == 0 {
		c.MaxStepDegs = 5.0
	}
}
