package sunposition

import (
	"context"
	"errors"
	"fmt"
	"image"
	"sync"

	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/data"
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
		resource.Registration[vision.Service, *Config]{Constructor: newService},
	)
}

type service struct {
	resource.Named
	resource.AlwaysRebuild

	logger logging.Logger
	cfg    *Config
	cam    camera.Camera

	formatLogOnce sync.Once
}

func newService(
	ctx context.Context,
	deps resource.Dependencies,
	conf resource.Config,
	logger logging.Logger,
) (vision.Service, error) {
	cfg, err := resource.NativeConfig[*Config](conf)
	if err != nil {
		return nil, err
	}
	return NewServiceWithConfig(ctx, deps, conf, cfg, logger)
}

// NewServiceWithConfig is the test-friendly + CLI-friendly entrypoint: caller
// has already converted to a typed *Config. Exported so cmd/cli/main.go can
// drive a smoke test without going through resource registration.
func NewServiceWithConfig(
	ctx context.Context,
	deps resource.Dependencies,
	conf resource.Config,
	cfg *Config,
	logger logging.Logger,
) (vision.Service, error) {
	cam, err := camera.FromDependencies(deps, cfg.Camera)
	if err != nil {
		return nil, fmt.Errorf("resolve camera %q: %w", cfg.Camera, err)
	}
	return &service{
		Named:  conf.ResourceName().AsNamed(),
		logger: logger,
		cfg:    cfg,
		cam:    cam,
	}, nil
}

// logFormatOnce records the camera's native image type the first time we see it.
// Operators can read this from logs to verify the YCbCr fast path is engaging.
func (s *service) logImageFormatOnce(img image.Image) {
	s.formatLogOnce.Do(func() {
		switch v := img.(type) {
		case *image.YCbCr:
			s.logger.Infow("camera native format: YCbCr (fast path)",
				"subsample", v.SubsampleRatio, "size", v.Rect.Size())
		case *image.Gray:
			s.logger.Info("camera native format: Gray (fast path)")
		default:
			s.logger.Warnw("camera not native YCbCr/Gray; using slow path",
				"type", fmt.Sprintf("%T", img))
		}
	})
}

func (s *service) grabImage(ctx context.Context) (image.Image, error) {
	imgs, _, err := s.cam.Images(ctx, nil, nil)
	if err != nil {
		return nil, err
	}
	if len(imgs) == 0 {
		return nil, errors.New("no images returned from camera")
	}
	return imgs[0].Image(ctx)
}

func (s *service) DetectionsFromCamera(
	ctx context.Context,
	cameraName string,
	extra map[string]interface{},
) ([]objdet.Detection, error) {
	img, err := s.grabImage(ctx)
	if err != nil {
		return nil, err
	}
	s.logImageFormatOnce(img)
	return s.detectionsForImage(img), nil
}

func (s *service) Detections(
	ctx context.Context,
	ni *camera.NamedImage,
	extra map[string]interface{},
) ([]objdet.Detection, error) {
	img, err := ni.Image(ctx)
	if err != nil {
		return nil, err
	}
	return s.detectionsForImage(img), nil
}

func (s *service) detectionsForImage(img image.Image) []objdet.Detection {
	tl, tr, bl, br, _ := quadrantError(img)
	return buildDetections(img.Bounds(), tl, tr, bl, br)
}

func (s *service) GetProperties(
	ctx context.Context,
	extra map[string]interface{},
) (*vision.Properties, error) {
	return &vision.Properties{
		ClassificationSupported: false,
		DetectionSupported:      true,
		ObjectPCDsSupported:     false,
	}, nil
}

func (s *service) CaptureAllFromCamera(
	ctx context.Context,
	cameraName string,
	opts viscapture.CaptureOptions,
	extra map[string]interface{},
) (viscapture.VisCapture, error) {
	img, err := s.grabImage(ctx)
	if err != nil {
		return viscapture.VisCapture{}, err
	}
	s.logImageFormatOnce(img)
	out := viscapture.VisCapture{}
	if opts.ReturnImage {
		ni, err := camera.NamedImageFromImage(img, cameraName, "", data.Annotations{})
		if err != nil {
			return viscapture.VisCapture{}, fmt.Errorf("wrap image for capture: %w", err)
		}
		out.Image = &ni
	}
	if opts.ReturnDetections {
		out.Detections = s.detectionsForImage(img)
	}
	return out, nil
}

func (s *service) Classifications(
	ctx context.Context,
	ni *camera.NamedImage,
	n int,
	extra map[string]interface{},
) (classification.Classifications, error) {
	return nil, errUnimplemented
}

func (s *service) ClassificationsFromCamera(
	ctx context.Context,
	cameraName string,
	n int,
	extra map[string]interface{},
) (classification.Classifications, error) {
	return nil, errUnimplemented
}

func (s *service) GetObjectPointClouds(
	ctx context.Context,
	cameraName string,
	extra map[string]interface{},
) ([]*vis.Object, error) {
	return nil, errUnimplemented
}

func (s *service) DoCommand(
	ctx context.Context,
	cmd map[string]interface{},
) (map[string]interface{}, error) {
	return nil, resource.ErrDoUnimplemented
}

func (s *service) Close(context.Context) error { return nil }
