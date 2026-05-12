package tracker

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.viam.com/rdk/components/servo"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/services/generic"
	"go.viam.com/rdk/services/vision"
)

var Model = resource.NewModel("devrel", "sun-tracker", "sun-servo-tracker")

func init() {
	resource.RegisterService(generic.API, Model,
		resource.Registration[resource.Resource, *Config]{Constructor: newService},
	)
}

type trackState struct {
	PanErr     float64
	TiltErr    float64
	PanDeg     uint32
	TiltDeg    uint32
	Brightness float64
	Locked     bool
	Enabled    bool
	LastUpdate time.Time
}

type service struct {
	resource.Named
	resource.AlwaysRebuild

	logger logging.Logger
	cfg    *Config

	visionSvc vision.Service
	pan       servo.Servo
	tilt      servo.Servo

	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu    sync.RWMutex
	state trackState

	// PD memory — written only inside step()
	lastPanErr  float64
	lastTiltErr float64
	lastTime    time.Time
}

func newService(
	ctx context.Context,
	deps resource.Dependencies,
	conf resource.Config,
	logger logging.Logger,
) (resource.Resource, error) {
	cfg, err := resource.NativeConfig[*Config](conf)
	if err != nil {
		return nil, err
	}
	applyDefaults(cfg)

	visionSvc, err := vision.FromDependencies(deps, cfg.VisionService)
	if err != nil {
		return nil, errors.Join(errors.New("resolving vision service"), err)
	}
	pan, err := servo.FromProvider(deps, cfg.PanServo)
	if err != nil {
		return nil, errors.Join(errors.New("resolving pan servo"), err)
	}
	tilt, err := servo.FromProvider(deps, cfg.TiltServo)
	if err != nil {
		return nil, errors.Join(errors.New("resolving tilt servo"), err)
	}

	loopCtx, cancel := context.WithCancel(context.Background())
	s := &service{
		Named:     conf.ResourceName().AsNamed(),
		logger:    logger,
		cfg:       cfg,
		visionSvc: visionSvc,
		pan:       pan,
		tilt:      tilt,
		cancel:    cancel,
	}
	s.state.Enabled = true

	s.wg.Add(1)
	go s.runLoop(loopCtx)
	return s, nil
}

// runLoop is implemented in Task 4.5. Empty body for now so the goroutine
// starts and exits immediately on context cancel.
func (s *service) runLoop(ctx context.Context) {
	defer s.wg.Done()
	<-ctx.Done()
}

func (s *service) Close(ctx context.Context) error {
	s.cancel()
	s.wg.Wait()
	return nil
}

func (s *service) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	return nil, resource.ErrDoUnimplemented
}
