package sunposition

import "errors"

type Config struct {
	Camera string `json:"camera"`
}

func (c *Config) Validate(path string) ([]string, []string, error) {
	if c.Camera == "" {
		return nil, nil, errors.New(path + ": camera is required")
	}
	return []string{c.Camera}, nil, nil
}
