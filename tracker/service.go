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

	visionSvc, err := vision.FromProvider(deps, cfg.VisionService)
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

func (s *service) runLoop(ctx context.Context) {
	defer s.wg.Done()
	period := time.Duration(float64(time.Second) / s.cfg.LoopHz)
	ticker := time.NewTicker(period)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.step(ctx, now)
		}
	}
}

func (s *service) step(ctx context.Context, now time.Time) {
	s.mu.RLock()
	enabled := s.state.Enabled
	prevPan := s.lastPanErr
	prevTilt := s.lastTiltErr
	prevTime := s.lastTime
	s.mu.RUnlock()
	if !enabled {
		return
	}

	dets, err := s.visionSvc.DetectionsFromCamera(ctx, s.cfg.Camera, nil)
	if err != nil {
		s.logger.Debugw("vision DetectionsFromCamera failed", "err", err)
		return
	}
	panErr, tiltErr, brightness, ok := detectionsToErrors(dets)
	if !ok {
		s.logger.Warnw("malformed detection set from vision service", "count", len(dets))
		brightness = 0
	}

	if brightness < s.cfg.MinBrightness {
		s.updateState(panErr, tiltErr, brightness, false, s.lastPan(), s.lastTilt())
		return
	}

	dt := now.Sub(prevTime).Seconds()
	if prevTime.IsZero() {
		dt = 1.0 / s.cfg.LoopHz
	}
	panStep := control(panErr, prevPan, dt, s.cfg.Kp, s.cfg.Kd, s.cfg.Deadband, s.cfg.MaxStepDegs)
	tiltStep := control(tiltErr, prevTilt, dt, s.cfg.Kp, s.cfg.Kd, s.cfg.Deadband, s.cfg.MaxStepDegs)

	locked := panStep == 0 && tiltStep == 0
	var panNow, tiltNow uint32
	if locked {
		panNow = s.tryReadServo(ctx, s.pan, s.lastPan())
		tiltNow = s.tryReadServo(ctx, s.tilt, s.lastTilt())
	} else {
		panNow, tiltNow = s.driveServos(ctx, panStep, tiltStep)
	}

	s.mu.Lock()
	s.lastPanErr = panErr
	s.lastTiltErr = tiltErr
	s.lastTime = now
	s.mu.Unlock()

	s.updateState(panErr, tiltErr, brightness, locked, panNow, tiltNow)
}

func (s *service) lastPan() uint32 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.PanDeg
}

func (s *service) lastTilt() uint32 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.TiltDeg
}

func (s *service) tryReadServo(ctx context.Context, sv servo.Servo, fallback uint32) uint32 {
	pos, err := sv.Position(ctx, nil)
	if err != nil {
		s.logger.Debugw("servo Position failed", "err", err)
		return fallback
	}
	return pos
}

func (s *service) driveServos(ctx context.Context, panStep, tiltStep float64) (uint32, uint32) {
	panNow := s.tryReadServo(ctx, s.pan, s.lastPan())
	tiltNow := s.tryReadServo(ctx, s.tilt, s.lastTilt())

	panTarget := stepClamp(panNow, panStep*float64(s.cfg.PanSign), s.cfg.PanMin, s.cfg.PanMax)
	tiltTarget := stepClamp(tiltNow, tiltStep*float64(s.cfg.TiltSign), s.cfg.TiltMin, s.cfg.TiltMax)

	if panTarget != panNow {
		if err := s.pan.Move(ctx, panTarget, nil); err != nil {
			s.logger.Debugw("pan Move failed", "err", err)
		} else {
			panNow = panTarget
		}
	}
	if tiltTarget != tiltNow {
		if err := s.tilt.Move(ctx, tiltTarget, nil); err != nil {
			s.logger.Debugw("tilt Move failed", "err", err)
		} else {
			tiltNow = tiltTarget
		}
	}
	return panNow, tiltNow
}

func (s *service) updateState(panErr, tiltErr, brightness float64, locked bool, panDeg, tiltDeg uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.PanErr = panErr
	s.state.TiltErr = tiltErr
	s.state.Brightness = brightness
	s.state.Locked = locked
	s.state.PanDeg = panDeg
	s.state.TiltDeg = tiltDeg
	s.state.LastUpdate = time.Now()
}

func (s *service) Close(ctx context.Context) error {
	s.cancel()
	s.wg.Wait()
	return nil
}

func (s *service) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	if _, ok := cmd["get_state"]; ok {
		s.mu.RLock()
		defer s.mu.RUnlock()
		return map[string]interface{}{
			"pan_error":   s.state.PanErr,
			"tilt_error":  s.state.TiltErr,
			"pan_deg":     s.state.PanDeg,
			"tilt_deg":    s.state.TiltDeg,
			"brightness":  s.state.Brightness,
			"locked":      s.state.Locked,
			"enabled":     s.state.Enabled,
			"last_update": s.state.LastUpdate.UnixMilli(),
		}, nil
	}
	if v, ok := cmd["enabled"]; ok {
		b, ok := v.(bool)
		if !ok {
			return nil, errors.New(`"enabled" must be a boolean`)
		}
		s.mu.Lock()
		s.state.Enabled = b
		s.mu.Unlock()
		return map[string]interface{}{"enabled": b}, nil
	}
	if _, ok := cmd["recenter"]; ok {
		s.mu.RLock()
		wasEnabled := s.state.Enabled
		s.mu.RUnlock()
		if wasEnabled {
			s.logger.Info("recenter while enabled — racing with control loop")
		}
		if err := s.pan.Move(ctx, 90, nil); err != nil {
			return nil, err
		}
		if err := s.tilt.Move(ctx, 90, nil); err != nil {
			return nil, err
		}
		return map[string]interface{}{"recentered": true}, nil
	}
	return nil, resource.ErrDoUnimplemented
}
