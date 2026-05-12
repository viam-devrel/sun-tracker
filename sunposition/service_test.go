package sunposition

import (
	"context"
	"image"
	"image/color"
	"testing"

	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/data"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	"go.viam.com/test"
)

// stubCamera implements just enough of camera.Camera for DetectionsFromCamera tests.
// Embeds camera.Camera for all unimplemented methods (panic on call); overrides
// Name() and Images() which are the only ones exercised by the tests.
type stubCamera struct {
	camera.Camera
	rname resource.Name
	img   image.Image
}

func newStubCamera(name string, img image.Image) *stubCamera {
	return &stubCamera{
		rname: resource.NewName(camera.API, name),
		img:   img,
	}
}

func (c *stubCamera) Name() resource.Name { return c.rname }

func (c *stubCamera) Images(
	ctx context.Context,
	filter []string,
	extra map[string]interface{},
) ([]camera.NamedImage, resource.ResponseMetadata, error) {
	ni, err := camera.NamedImageFromImage(c.img, "", "", data.Annotations{})
	if err != nil {
		return nil, resource.ResponseMetadata{}, err
	}
	return []camera.NamedImage{ni}, resource.ResponseMetadata{}, nil
}

func (c *stubCamera) Close(ctx context.Context) error { return nil }

func newRGBAWithBrightTR() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 50; y++ {
		for x := 50; x < 100; x++ {
			img.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	return img
}

func TestDetectionsFromCamera_FourInFixedOrder(t *testing.T) {
	logger := logging.NewTestLogger(t)
	cam := newStubCamera("my_camera", newRGBAWithBrightTR())
	deps := resource.Dependencies{cam.Name(): cam}

	conf := resource.Config{Name: "sun_vision", Model: Model}
	cfg := &Config{Camera: "my_camera"}

	svc, err := NewServiceWithConfig(context.Background(), deps, conf, cfg, logger)
	test.That(t, err, test.ShouldBeNil)
	defer svc.Close(context.Background())

	dets, err := svc.DetectionsFromCamera(context.Background(), "my_camera", nil)
	test.That(t, err, test.ShouldBeNil)
	test.That(t, len(dets), test.ShouldEqual, 4)

	labels := []string{
		dets[0].Label(), dets[1].Label(), dets[2].Label(), dets[3].Label(),
	}
	test.That(t, labels, test.ShouldResemble,
		[]string{"top-left", "top-right", "bottom-left", "bottom-right"})

	// Top-right should have the highest score.
	trScore := dets[1].Score()
	for i := 0; i < 4; i++ {
		if i == 1 {
			continue
		}
		test.That(t, trScore, test.ShouldBeGreaterThan, dets[i].Score())
	}
}

func TestGetProperties_DetectionOnly(t *testing.T) {
	logger := logging.NewTestLogger(t)
	cam := newStubCamera("my_camera", newRGBAWithBrightTR())
	deps := resource.Dependencies{cam.Name(): cam}
	conf := resource.Config{Name: "sun_vision", Model: Model}
	cfg := &Config{Camera: "my_camera"}

	svc, err := NewServiceWithConfig(context.Background(), deps, conf, cfg, logger)
	test.That(t, err, test.ShouldBeNil)

	props, err := svc.GetProperties(context.Background(), nil)
	test.That(t, err, test.ShouldBeNil)
	test.That(t, props.DetectionSupported, test.ShouldBeTrue)
	test.That(t, props.ClassificationSupported, test.ShouldBeFalse)
	test.That(t, props.ObjectPCDsSupported, test.ShouldBeFalse)
}
