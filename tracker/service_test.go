package tracker

import (
	"context"
	"image"
	"sync"
	"testing"
	"time"

	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/components/servo"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	vision "go.viam.com/rdk/services/vision"
	"go.viam.com/rdk/vision/classification"
	vis "go.viam.com/rdk/vision"
	objdet "go.viam.com/rdk/vision/objectdetection"
	"go.viam.com/rdk/vision/viscapture"
	"go.viam.com/test"
)

// stubVision returns a configurable detection set on every DetectionsFromCamera call.
type stubVision struct {
	resource.Named
	resource.TriviallyCloseable
	resource.TriviallyReconfigurable

	mu        sync.Mutex
	dets      []objdet.Detection
	callCount int
}

func newStubVision(name string) *stubVision {
	return &stubVision{Named: resource.NewName(vision.API, name).AsNamed()}
}

func (v *stubVision) setDets(dets []objdet.Detection) {
	v.mu.Lock()
	v.dets = dets
	v.mu.Unlock()
}

func (v *stubVision) DetectionsFromCamera(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objdet.Detection, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.callCount++
	out := make([]objdet.Detection, len(v.dets))
	copy(out, v.dets)
	return out, nil
}

// Implement the rest of vision.Service with empty returns. Signatures match
// services/vision/vision.go exactly.
func (v *stubVision) Detections(ctx context.Context, img *camera.NamedImage, extra map[string]interface{}) ([]objdet.Detection, error) {
	return nil, nil
}
func (v *stubVision) ClassificationsFromCamera(ctx context.Context, cameraName string, n int, extra map[string]interface{}) (classification.Classifications, error) {
	return nil, nil
}
func (v *stubVision) Classifications(ctx context.Context, img *camera.NamedImage, n int, extra map[string]interface{}) (classification.Classifications, error) {
	return nil, nil
}
func (v *stubVision) GetObjectPointClouds(ctx context.Context, cameraName string, extra map[string]interface{}) ([]*vis.Object, error) {
	return nil, nil
}
func (v *stubVision) GetProperties(ctx context.Context, extra map[string]interface{}) (*vision.Properties, error) {
	return &vision.Properties{}, nil
}
func (v *stubVision) CaptureAllFromCamera(ctx context.Context, cameraName string, opts viscapture.CaptureOptions, extra map[string]interface{}) (viscapture.VisCapture, error) {
	return viscapture.VisCapture{}, nil
}
func (v *stubVision) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	return nil, nil
}

// stubServo records every Move call and reports a settable position.
type stubServo struct {
	resource.Named
	resource.TriviallyCloseable
	resource.TriviallyReconfigurable

	mu    sync.Mutex
	pos   uint32
	moves []uint32
}

func newStubServo(name string, initial uint32) *stubServo {
	return &stubServo{
		Named: resource.NewName(servo.API, name).AsNamed(),
		pos:   initial,
	}
}

func (s *stubServo) Move(ctx context.Context, deg uint32, extra map[string]interface{}) error {
	s.mu.Lock()
	s.pos = deg
	s.moves = append(s.moves, deg)
	s.mu.Unlock()
	return nil
}

func (s *stubServo) Position(ctx context.Context, extra map[string]interface{}) (uint32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pos, nil
}

func (s *stubServo) Stop(ctx context.Context, extra map[string]interface{}) error { return nil }
func (s *stubServo) IsMoving(ctx context.Context) (bool, error)                   { return false, nil }
func (s *stubServo) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	return nil, nil
}

func (s *stubServo) moveCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.moves)
}

// buildTestService wires stubs into a *service for direct step() testing.
// Does NOT start the goroutine — tests drive step() directly.
func buildTestService(t *testing.T, vis *stubVision, pan, tilt *stubServo) *service {
	t.Helper()
	cfg := &Config{VisionService: "v", Camera: "c", PanServo: "p", TiltServo: "t"}
	applyDefaults(cfg)
	s := &service{
		logger:    logging.NewTestLogger(t),
		cfg:       cfg,
		visionSvc: vis,
		pan:       pan,
		tilt:      tilt,
	}
	s.state.Enabled = true
	return s
}

func brightTRDets(t *testing.T) []objdet.Detection {
	t.Helper()
	bounds := image.Rect(0, 0, 100, 100)
	return []objdet.Detection{
		objdet.NewDetection(bounds, image.Rect(0, 0, 50, 50), 0.05, "top-left"),
		objdet.NewDetection(bounds, image.Rect(50, 0, 100, 50), 0.80, "top-right"),
		objdet.NewDetection(bounds, image.Rect(0, 50, 50, 100), 0.05, "bottom-left"),
		objdet.NewDetection(bounds, image.Rect(50, 50, 100, 100), 0.10, "bottom-right"),
	}
}

func TestStep_BrightInTopRight_MovesPanRight(t *testing.T) {
	vis := newStubVision("v")
	pan := newStubServo("p", 90)
	tilt := newStubServo("t", 90)
	vis.setDets(brightTRDets(t))
	s := buildTestService(t, vis, pan, tilt)

	s.step(context.Background(), time.Now())

	test.That(t, pan.moveCount(), test.ShouldBeGreaterThan, 0)
	s.mu.RLock()
	pe := s.state.PanErr
	s.mu.RUnlock()
	test.That(t, pe, test.ShouldBeGreaterThan, 0.0)
}

func TestDoCommand_GetState(t *testing.T) {
	vis := newStubVision("v")
	pan := newStubServo("p", 90)
	tilt := newStubServo("t", 90)
	vis.setDets(brightTRDets(t))
	s := buildTestService(t, vis, pan, tilt)

	s.step(context.Background(), time.Now())

	out, err := s.DoCommand(context.Background(), map[string]interface{}{"get_state": true})
	test.That(t, err, test.ShouldBeNil)
	test.That(t, out["enabled"], test.ShouldEqual, true)
	test.That(t, out["pan_error"].(float64), test.ShouldBeGreaterThan, 0.0)
	_, ok := out["last_update"]
	test.That(t, ok, test.ShouldBeTrue)
}

func TestDoCommand_EnabledFalse_StopsMoves(t *testing.T) {
	vis := newStubVision("v")
	pan := newStubServo("p", 90)
	tilt := newStubServo("t", 90)
	vis.setDets(brightTRDets(t))
	s := buildTestService(t, vis, pan, tilt)

	_, err := s.DoCommand(context.Background(), map[string]interface{}{"enabled": false})
	test.That(t, err, test.ShouldBeNil)

	before := pan.moveCount()
	s.step(context.Background(), time.Now())
	after := pan.moveCount()
	test.That(t, after, test.ShouldEqual, before)
}

func TestDoCommand_Recenter(t *testing.T) {
	vis := newStubVision("v")
	pan := newStubServo("p", 30)
	tilt := newStubServo("t", 30)
	s := buildTestService(t, vis, pan, tilt)

	out, err := s.DoCommand(context.Background(), map[string]interface{}{"recenter": true})
	test.That(t, err, test.ShouldBeNil)
	test.That(t, out["recentered"], test.ShouldEqual, true)
	pos, _ := pan.Position(context.Background(), nil)
	test.That(t, pos, test.ShouldEqual, uint32(90))
	pos, _ = tilt.Position(context.Background(), nil)
	test.That(t, pos, test.ShouldEqual, uint32(90))
}

func TestDoCommand_UnknownReturnsUnimplemented(t *testing.T) {
	vis := newStubVision("v")
	pan := newStubServo("p", 90)
	tilt := newStubServo("t", 90)
	s := buildTestService(t, vis, pan, tilt)

	_, err := s.DoCommand(context.Background(), map[string]interface{}{"frobnicate": "yes"})
	test.That(t, err, test.ShouldEqual, resource.ErrDoUnimplemented)
}
