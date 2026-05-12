package tracker

import "errors"

type Config struct {
	VisionService string `json:"vision_service"`
	Camera        string `json:"camera"`
	PanServo      string `json:"pan_servo"`
	TiltServo     string `json:"tilt_servo"`
}

func (c *Config) Validate(path string) ([]string, []string, error) {
	if c.VisionService == "" || c.Camera == "" || c.PanServo == "" || c.TiltServo == "" {
		return nil, nil, errors.New(path + ": vision_service, camera, pan_servo, tilt_servo are required")
	}
	return []string{c.VisionService, c.Camera, c.PanServo, c.TiltServo}, nil, nil
}
