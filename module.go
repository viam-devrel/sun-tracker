package suntracker

import (
  vision "go.viam.com/rdk/services/vision"
  "bytes"
"context"
"fmt"
"image"
"github.com/pkg/errors"
commonpb "go.viam.com/api/common/v1"
pb "go.viam.com/api/service/vision/v1"
"go.viam.com/utils/protoutils"
"go.viam.com/utils/rpc"
"go.viam.com/utils/trace"
"go.viam.com/rdk/components/camera"
"go.viam.com/rdk/data"
"go.viam.com/rdk/logging"
"go.viam.com/rdk/pointcloud"
rprotoutils "go.viam.com/rdk/protoutils"
"go.viam.com/rdk/resource"
"go.viam.com/rdk/utils"
vis "go.viam.com/rdk/vision"
"go.viam.com/rdk/vision/classification"
objdet "go.viam.com/rdk/vision/objectdetection"
"go.viam.com/rdk/vision/viscapture"
)

var (
	SunPosition = resource.NewModel("devrel", "sun-tracker", "sun-position")
	errUnimplemented = errors.New("unimplemented")
)

func init() {
	resource.RegisterService(vision.API, SunPosition,
		resource.Registration[vision.Service, *Config]{
			Constructor: newSunTrackerSunPosition,
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
//   1. Required dependencies: other resources that must exist for this resource to work.
//   2. Optional dependencies: other resources that may exist but are not required.
//   3. An error if any Config fields are missing or invalid.
//
// The `path` parameter indicates
// where this resource appears in the machine's JSON configuration
// (for example, "components.0"). You can use it in error messages 
// to indicate which resource has a problem.
func (cfg *Config) Validate(path string) ([]string, []string, error) {
	// Add config validation code here
	 return nil, nil, nil
}

type sunTrackerSunPosition struct {
	resource.AlwaysRebuild
	resource.Named

	name   resource.Name

	logger logging.Logger
	cfg    *Config

	cancelCtx  context.Context
	cancelFunc func()
}

func newSunTrackerSunPosition(ctx context.Context, deps resource.Dependencies, rawConf resource.Config, logger logging.Logger) (vision.Service, error) {
	conf, err := resource.NativeConfig[*Config](rawConf)
	if err != nil {
		return nil, err
	}

    return NewSunPosition(ctx, deps, rawConf.ResourceName(), conf, logger)

}

func NewSunPosition(ctx context.Context, deps resource.Dependencies, name resource.Name, conf *Config, logger logging.Logger) (vision.Service, error) {

	cancelCtx, cancelFunc := context.WithCancel(context.Background())

	s := &sunTrackerSunPosition{
		name:       name,
		logger:     logger,
		cfg:        conf,
		cancelCtx:  cancelCtx,
		cancelFunc: cancelFunc,
	}
	return s, nil
}

func (s *sunTrackerSunPosition) Name() resource.Name {
	return s.name
}

// DetectionsFromCamera returns a list of detections from the next image from a specified camera using a configured detector.
func (s *sunTrackerSunPosition) DetectionsFromCamera(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objdet.Detection, error) {
	return nil, fmt.Errorf("not implemented")
}

 // Detections returns a list of detections from a given image using a configured detector.
func (s *sunTrackerSunPosition) Detections(ctx context.Context, img *camera.NamedImage, extra map[string]interface{}) ([]objdet.Detection, error) {
	return nil, fmt.Errorf("not implemented")
}

 // ClassificationsFromCamera returns a list of classifications from the next image from a specified camera using a configured classifier.
func (s *sunTrackerSunPosition) ClassificationsFromCamera(ctx context.Context, cameraName string, n int, extra map[string]interface{}) (classification.Classifications, error) {
	var classificationsRetVal classification.Classifications

	return classificationsRetVal, fmt.Errorf("not implemented")
}

 // Classifications returns a list of classifications from a given image using a configured classifier.
func (s *sunTrackerSunPosition) Classifications(ctx context.Context, img *camera.NamedImage, n int, extra map[string]interface{}) (classification.Classifications, error) {
	var classificationsRetVal classification.Classifications

	return classificationsRetVal, fmt.Errorf("not implemented")
}

 // GetObjectPointClouds returns a list of 3D point cloud objects and metadata from the latest 3D camera image using a specified segmenter.
func (s *sunTrackerSunPosition) GetObjectPointClouds(ctx context.Context, cameraName string, extra map[string]interface{}) ([]*vis.Object, error) {
	return nil, fmt.Errorf("not implemented")
}

 // properties
func (s *sunTrackerSunPosition) GetProperties(ctx context.Context, extra map[string]interface{}) (*vision.Properties, error) {
	return nil, fmt.Errorf("not implemented")
}

 // CaptureAllFromCamera returns the next image, detections, classifications, and objects all together, given a camera name. Used for
// visualization.
func (s *sunTrackerSunPosition) CaptureAllFromCamera(ctx context.Context, cameraName string, captureOptions viscapture.CaptureOptions, extra map[string]interface{}) (viscapture.VisCapture, error) {
	var visCaptureRetVal viscapture.VisCapture

	return visCaptureRetVal, fmt.Errorf("not implemented")
}

 func (s *sunTrackerSunPosition) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	return nil, fmt.Errorf("not implemented")
}

 func (s *sunTrackerSunPosition) Status(ctx context.Context) (map[string]interface{}, error) {
	return nil, fmt.Errorf("not implemented")
}



func (s *sunTrackerSunPosition) Close(context.Context) error {
	// Put close code here
	s.cancelFunc()
	return nil
}
