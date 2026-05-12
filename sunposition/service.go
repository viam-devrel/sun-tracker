package sunposition

import (
	"context"

	"github.com/pkg/errors"
	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	vision "go.viam.com/rdk/services/vision"
	vis "go.viam.com/rdk/vision"
	"go.viam.com/rdk/vision/classification"
	objdet "go.viam.com/rdk/vision/objectdetection"
	"go.viam.com/rdk/vision/viscapture"
)

var (
	Model            = resource.NewModel("devrel", "sun-tracker", "sun-position")
	errUnimplemented = errors.New("unimplemented")
)

func init() {
	resource.RegisterService(vision.API, Model,
		resource.Registration[vision.Service, *Config]{
			Constructor: newService,
		},
	)
}

type Config struct {
	/*
		Put config attributes here. There should be public/exported fields
		with a `json` parameter at the end of each attribute.

		Example config struct:
			type Config struct {
				Pin   string `json:"pin"`
				Board string `json:"board"`
				MinDeg *float64 `json:"min_angle_deg,omitempty"`
			}

		If your model does not need a config, replace *Config in the init
		function with resource.NoNativeConfig
	*/
}

// Validate ensures all parts of the config are valid and important fields exist.
// Returns three values:
//  1. Required dependencies: other resources that must exist for this resource to work.
//  2. Optional dependencies: other resources that may exist but are not required.
//  3. An error if any Config fields are missing or invalid.
//
// The `path` parameter indicates
// where this resource appears in the machine's JSON configuration
// (for example, "components.0"). You can use it in error messages
// to indicate which resource has a problem.
func (cfg *Config) Validate(path string) ([]string, []string, error) {
	// Add config validation code here
	return nil, nil, nil
}

type service struct {
	resource.AlwaysRebuild
	resource.Named

	name resource.Name

	logger logging.Logger
	cfg    *Config

	cancelCtx  context.Context
	cancelFunc func()
}

func newService(ctx context.Context, deps resource.Dependencies, rawConf resource.Config, logger logging.Logger) (vision.Service, error) {
	conf, err := resource.NativeConfig[*Config](rawConf)
	if err != nil {
		return nil, err
	}

	return NewService(ctx, deps, rawConf.ResourceName(), conf, logger)
}

func NewService(ctx context.Context, deps resource.Dependencies, name resource.Name, conf *Config, logger logging.Logger) (vision.Service, error) {
	cancelCtx, cancelFunc := context.WithCancel(context.Background())

	s := &service{
		name:       name,
		logger:     logger,
		cfg:        conf,
		cancelCtx:  cancelCtx,
		cancelFunc: cancelFunc,
	}
	return s, nil
}

func (s *service) Name() resource.Name {
	return s.name
}

// DetectionsFromCamera returns a list of detections from the next image from a specified camera using a configured detector.
func (s *service) DetectionsFromCamera(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objdet.Detection, error) {
	return nil, errUnimplemented
}

// Detections returns a list of detections from a given image using a configured detector.
func (s *service) Detections(ctx context.Context, img *camera.NamedImage, extra map[string]interface{}) ([]objdet.Detection, error) {
	return nil, errUnimplemented
}

// ClassificationsFromCamera returns a list of classifications from the next image from a specified camera using a configured classifier.
func (s *service) ClassificationsFromCamera(ctx context.Context, cameraName string, n int, extra map[string]interface{}) (classification.Classifications, error) {
	var classificationsRetVal classification.Classifications

	return classificationsRetVal, errUnimplemented
}

// Classifications returns a list of classifications from a given image using a configured classifier.
func (s *service) Classifications(ctx context.Context, img *camera.NamedImage, n int, extra map[string]interface{}) (classification.Classifications, error) {
	var classificationsRetVal classification.Classifications

	return classificationsRetVal, errUnimplemented
}

// GetObjectPointClouds returns a list of 3D point cloud objects and metadata from the latest 3D camera image using a specified segmenter.
func (s *service) GetObjectPointClouds(ctx context.Context, cameraName string, extra map[string]interface{}) ([]*vis.Object, error) {
	return nil, errUnimplemented
}

// properties
func (s *service) GetProperties(ctx context.Context, extra map[string]interface{}) (*vision.Properties, error) {
	return nil, errUnimplemented
}

// CaptureAllFromCamera returns the next image, detections, classifications, and objects all together, given a camera name. Used for
// visualization.
func (s *service) CaptureAllFromCamera(ctx context.Context, cameraName string, captureOptions viscapture.CaptureOptions, extra map[string]interface{}) (viscapture.VisCapture, error) {
	var visCaptureRetVal viscapture.VisCapture

	return visCaptureRetVal, errUnimplemented
}

func (s *service) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	return nil, errUnimplemented
}

func (s *service) Status(ctx context.Context) (map[string]interface{}, error) {
	return nil, errUnimplemented
}

func (s *service) Close(context.Context) error {
	// Put close code here
	s.cancelFunc()
	return nil
}
