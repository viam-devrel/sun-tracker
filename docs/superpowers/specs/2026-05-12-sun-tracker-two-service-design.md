# Sun Tracker — Two-Service Design

A Viam Go module that drives a pan/tilt servo pair toward the brightest source in a camera's field of view, split into two cooperating models:

- **`devrel:sun-tracker:sun-position`** — `rdk:service:vision` that emits per-quadrant brightness detections from a camera frame.
- **`devrel:sun-tracker:sun-servo-tracker`** — `rdk:service:generic` that owns the closed control loop: polls the vision service at `LoopHz`, derives PD control, and commands two servos.

This spec supersedes the single-component design at `.plans/sun-tracker-design.md`. The original control-law math (§4), YCbCr fast path (§5), tuning recipe (§9), verification checklist (§10), and phased build plan (§11) are largely preserved — items marked **VERIFY** must still be resolved against the user's vendored RDK version before relying on them.

---

## 1. Context and motivation

**Hardware:** Raspberry Pi 5, USB camera, two hobby servos (pan + tilt), small 5V solar panel, INA219 power sensor, USB-PD battery pack as load.

**Demo goal:** Show a solar panel tracking a movable indoor "sun" (handheld flashlight or motorized spotlight) in real time, with live power-output telemetry demonstrating that tracking improves harvest vs. a static panel. Targets both social-media video and an unattended booth demo.

**Why two services instead of one sensor:** The original `.plans/sun-tracker-design.md` proposed packaging everything (camera read, image processing, PD control, servo drive) into a single `rdk:component:sensor`. That works, but it conflates image processing with control. Splitting into a vision service + a generic control service buys:

- The vision service is reusable for any "where is the bright thing" use case (e.g. a future ML-based detector that swaps in behind the same `vision.Service` interface).
- The control service is reusable for any quadrant-driven pan/tilt task (not just sun-following).
- Detection results capture into data manager via the standard `vision.Service` capture path, with no Readings-coupling.
- The two services are independently testable.

**Why per-quadrant detections (and not, say, a single brightest-blob bounding box):** The control law in §4 of the original doc balances four quadrant sums. That math is robust, parameter-light, and explainable — exactly what we want for a demo. Per-quadrant detections preserve that algorithm verbatim while still fitting the standard `vision.Service` contract.

---

## 2. Goals and non-goals

**In scope**
- Custom Go module with two registered models in package `github.com/viam-devrel/sun-tracker`.
- Vision service computing per-quadrant luma on each requested frame, returning four `objdet.Detection`s with stable labels.
- Generic tracker service running a background control loop: polls vision, derives PD-controlled servo steps, drives pan and tilt servos, exposes state via `DoCommand`.
- YCbCr fast path for image processing, with `Gray` and generic fallbacks.
- `DoCommand` verbs: `get_state`, `enabled`, `recenter`.
- Graceful low-light behavior: brightness floor freezes servos rather than chasing noise.
- Unit tests for image-processing, PD math, and control-loop behavior (with stub camera / stub vision / stub servos).

**Out of scope**
- Ephemeris-based tracking (irrelevant indoors).
- ML-based sun detection (could be slotted in later as an alternative vision-service model — explicitly *not* part of this spec).
- Power-sensor integration (separate component, already exists).
- Indoor "sun" simulation hardware.
- Dashboard / web UI.
- Live re-tuning via `DoCommand` (rebuild on config change is acceptable).
- Hot reconfigure: both services use `resource.AlwaysRebuild`.

---

## 3. Architecture

### 3.1 Topology

```
                       camera (dep)
                            │
                            ▼
       devrel:sun-tracker:sun-position  (vision.Service)
                            │
            DetectionsFromCamera(camera_name) → [4 detections]
                            │
                            ▼
   devrel:sun-tracker:sun-servo-tracker  (generic.Service)
                            │
         PD control + deadband + brightness floor
                            │
                  ┌─────────┴─────────┐
                  ▼                   ▼
              pan_servo            tilt_servo
                  │                   │
                  └──── Move(deg) ────┘

  data manager  ─────►  DoCommand({"get_state": true}) @ 5Hz
                ─────►  CaptureAllFromCamera (vision-side capture)
```

Vision service holds the camera dependency. The tracker holds the vision-service dependency plus the two servo dependencies. The camera is never referenced by the tracker except by name — the tracker passes the configured `camera` name through `DetectionsFromCamera`.

### 3.2 Resource declarations

| Model | API | Required deps | Purpose |
|---|---|---|---|
| `devrel:sun-tracker:sun-position` | `rdk:service:vision` | `{ camera }` | Compute per-quadrant brightness → 4 detections |
| `devrel:sun-tracker:sun-servo-tracker` | `rdk:service:generic` | `{ vision_service, pan_servo, tilt_servo }` | Control loop |

### 3.3 Threading model

**Vision service** is request-driven: no background goroutine. `DetectionsFromCamera` grabs an image, computes quadrants, returns. Stateless across calls aside from the cached camera handle.

**Tracker service** owns the only background goroutine:

```
Constructor
   └── starts background goroutine (loopCtx, cancelable)
        └── ticker @ LoopHz
             ├── visionSvc.DetectionsFromCamera(camera_name)
             ├── parse 4 detections → pan_err, tilt_err, brightness
             ├── brightness floor check (skip move if too dark)
             ├── PD control → per-axis step
             ├── drive servos (blocking Move call)
             └── update shared state (mutex)

DoCommand
   └── read/write shared state under RWMutex

Close
   └── cancel loop context, wait for goroutine via WaitGroup
```

`prevTime` / `prevPanErr` / `prevTiltErr` live as loop-goroutine-local variables, not in shared state — single writer, no lock.

---

## 4. Control approach

(Unchanged from `.plans/sun-tracker-design.md` §4, restated here for self-contained reading.)

### 4.1 Quadrant error derivation

The image is divided into four equal quadrants:

```
┌───────┬───────┐
│  TL   │  TR   │
├───────┼───────┤
│  BL   │  BR   │
└───────┴───────┘
```

The vision service returns four `Detection`s, one per quadrant, with `Score` = mean luma of that quadrant divided by 255 (so `Score ∈ [0, 1]` is a brightness proxy, not a confidence in the conventional sense — see §5.2).

The tracker derives:

```
total     = score_TL + score_TR + score_BL + score_BR
pan_err   = (score_TR + score_BR − score_TL − score_BL) / total      # ∈ [−1, +1], + means light is right
tilt_err  = (score_TL + score_TR − score_BL − score_BR) / total      # ∈ [−1, +1], + means light is up
brightness = (total / 4) × 255                                       # mean luma 0–255
```

Normalizing by `total` makes `pan_err` / `tilt_err` invariant to absolute brightness. A dim flashlight in one quadrant produces the same error magnitude as a bright one — what matters is the imbalance fraction.

### 4.2 Control law

Per-axis PD on the imbalance fraction:

```
correction_deg = Kp · err + Kd · (err − prev_err) / dt
step           = clamp(correction_deg, −MaxStep, +MaxStep)
target         = clamp(current + sign · step, axis_min, axis_max)
```

`sign` per axis flips direction without rewiring (depends on physical servo orientation relative to camera).

Inside the deadband (`|err| < Deadband`), `step = 0` — prevents jitter when centered. When *both* steps are 0 the tracker marks `locked = true` and skips the `Move` calls entirely.

### 4.3 Brightness floor

If `brightness < MinBrightness`, skip the move entirely and update state with `locked = false`, `enabled = true`. Prevents the tracker from chasing the brightest ambient patch when there's no real "sun" in frame.

---

## 5. Vision service: `sun-position`

### 5.1 Config

```go
type Config struct {
    Camera string `json:"camera"`
}
```

`Validate(path)` returns `[]string{cfg.Camera}, nil, nil` if `Camera != ""`; otherwise an error including `path`.

### 5.2 Detection contract

Every call to `DetectionsFromCamera` or `Detections` returns **exactly four** detections in fixed order: TL, TR, BL, BR.

For each:
- `BoundingBox`: `image.Rect(...)` covering that quadrant in source pixel coordinates.
- `Score`: mean luma of the quadrant, divided by 255. Brightness proxy ∈ [0, 1], not a conventional confidence.
- `Label` and `ClassName`: stable identifiers — `"top-left"`, `"top-right"`, `"bottom-left"`, `"bottom-right"`. The tracker matches on these.

Four detections are returned even in pitch black (all scores ≈ 0). The tracker decides what to do with that — the vision service's job is "report what you see."

### 5.3 Methods

| Method | Behavior |
|---|---|
| `DetectionsFromCamera(ctx, cameraName, extra)` | Resolve camera (from deps cached at constructor), grab image, dispatch to quadrant computation, return 4 detections. |
| `Detections(ctx, img, extra)` | Same dispatch on the passed-in image — no camera read. |
| `CaptureAllFromCamera(ctx, cameraName, opts, extra)` | Returns the image plus the 4 detections. Used by data manager for visualization. |
| `GetProperties(ctx, extra)` | Returns `vision.Properties{DetectionSupported: true, ClassificationSupported: false, ObjectPCDsSupported: false}`. |
| `Classifications`, `ClassificationsFromCamera`, `GetObjectPointClouds` | Return `errUnimplemented` sentinel. |
| `DoCommand(ctx, cmd)` | Returns `resource.ErrDoUnimplemented`. |
| `Name()`, `Close(ctx)` | Standard. `Close` cancels the constructor context (vision service has no goroutine, so no WaitGroup). |

### 5.4 Image-processing fast path

Dispatch by concrete image type:

```go
func quadrantError(img image.Image) (tl, tr, bl, br, brightness float64) {
    switch t := img.(type) {
    case *image.YCbCr: return quadrantYCbCr(t)
    case *image.Gray:  return quadrantGray(t)
    default:           return quadrantGeneric(img)
    }
}
```

`quadrantYCbCr` indexes the `Y` plane directly using `YStride`, subtracting `Rect.Min` to support SubImage views. Skips interface dispatch, allocation, and YCbCr→RGB conversion.

`quadrantGray` is the same shape against `img.Pix` and `img.Stride`.

`quadrantGeneric` is the slow fallback using `img.At(x, y).RGBA()` per pixel with BT.601 luma weighting (0.299R + 0.587G + 0.114B). Used if a transform pipeline re-encodes to RGB.

At startup (one-shot from a goroutine launched in the constructor), grab one image and log its concrete type at `Info` level so operators know whether the fast path is engaging. If a transform pipeline forces RGB, the log line surfaces it.

Expected cost on 640×480 luma at Pi 5, single thread, native YCbCr path: well under 5 ms. At a 10 Hz tracker loop rate, image processing is a small fraction of the budget; servo travel time dominates.

---

## 6. Tracker service: `sun-servo-tracker`

### 6.1 Config

```go
type Config struct {
    VisionService string `json:"vision_service"`
    Camera        string `json:"camera"`        // name passed to DetectionsFromCamera
    PanServo      string `json:"pan_servo"`
    TiltServo     string `json:"tilt_servo"`

    Kp            float64 `json:"kp,omitempty"`
    Kd            float64 `json:"kd,omitempty"`
    Deadband      float64 `json:"deadband,omitempty"`
    MinBrightness float64 `json:"min_brightness,omitempty"` // 0–255

    PanSign       int     `json:"pan_sign,omitempty"`
    TiltSign      int     `json:"tilt_sign,omitempty"`
    PanMin        uint32  `json:"pan_min,omitempty"`
    PanMax        uint32  `json:"pan_max,omitempty"`
    TiltMin       uint32  `json:"tilt_min,omitempty"`
    TiltMax       uint32  `json:"tilt_max,omitempty"`

    LoopHz        float64 `json:"loop_hz,omitempty"`
    MaxStepDegs   float64 `json:"max_step_degs,omitempty"`
}
```

`Validate(path)` requires `vision_service`, `camera`, `pan_servo`, `tilt_servo`; returns all four as required deps. Optional fields get defaults applied at constructor time:

| Field | Default |
|---|---|
| `Kp` | 8.0 |
| `Kd` | 0.0 |
| `Deadband` | 0.05 |
| `MinBrightness` | 30.0 |
| `PanSign`, `TiltSign` | +1 |
| `PanMin`, `TiltMin` | 0 |
| `PanMax`, `TiltMax` | 180 |
| `LoopHz` | 10 |
| `MaxStepDegs` | 5.0 |

The `Camera` field carries the camera **name** even though the tracker doesn't depend on the camera directly: `DetectionsFromCamera` is keyed by name, and we'd rather have a redundant config string than teach the tracker to introspect the vision service's config.

### 6.2 Control loop tick

Per-tick logic, extracted into a private `step(ctx, now)` method so it's testable without a real ticker:

1. Read `enabled` under `RLock`. If false, return.
2. Call `visionSvc.DetectionsFromCamera(ctx, cfg.Camera, nil)`. On error, log Debug and return (no state change beyond a tiny structured log).
3. Parse the 4 detections by `ClassName` into `tl`, `tr`, `bl`, `br`. If detection count ≠ 4 or any expected label is missing, log at Warn (rate-limited dedupe) and treat as brightness = 0.
4. Compute `total`, `pan_err`, `tilt_err`, `brightness` per §4.1.
5. If `brightness < cfg.MinBrightness`: update state (`pan_err`, `tilt_err`, `brightness`, `locked=false`, current angles), return.
6. Compute `dt = now − prevTime`. First call: `dt = 1.0 / LoopHz`. Update `prevTime`.
7. `panStep = control(pan_err, prevPanErr, dt, Kp, Kd, Deadband, MaxStepDegs)` — likewise `tiltStep`. Update `prevPanErr`, `prevTiltErr`.
8. `locked = (panStep == 0 && tiltStep == 0)`. If locked, skip `Move`; still read current positions for state. Otherwise compute new targets and call `Move` per axis.
9. Update state under `Lock`.

### 6.3 DoCommand verbs

| Command | Effect | Return |
|---|---|---|
| `{"get_state": true}` | Snapshot of state under `RLock`. | `{pan_error, tilt_error, pan_deg, tilt_deg, brightness, locked, enabled, last_update}` (`last_update` as `UnixMilli`). |
| `{"enabled": <bool>}` | Set enabled flag under `Lock`. Mid-tick callers complete normally; next tick sees the change. | `{"enabled": v}`. |
| `{"recenter": true}` | Drive both servos to 90°. Logs Info if `enabled == true` (recenter races the loop, accepted as a debug verb). | `{"recentered": true}`. |
| Anything else | `resource.ErrDoUnimplemented`. |

### 6.4 Recommended data-capture config

Consumer-side, on the tracker resource in the robot config:

```json
{
  "service_configs": [{
    "type": "data_manager",
    "attributes": {
      "capture_methods": [{
        "method": "DoCommand",
        "additional_params": {"command": {"get_state": true}},
        "capture_frequency_hz": 5
      }]
    }
  }]
}
```

**VERIFY** — the exact key for passing a command payload through data-manager capture of `DoCommand` on a generic service has shifted across RDK versions. Resolve against the vendored RDK before publishing.

The vision service is captured separately via its standard `CaptureAllFromCamera` path:

```json
{
  "service_configs": [{
    "type": "data_manager",
    "attributes": {
      "capture_methods": [{
        "method": "CaptureAllFromCamera",
        "capture_frequency_hz": 5
      }]
    }
  }]
}
```

### 6.5 Lifecycle

- `resource.AlwaysRebuild` — config changes tear down and rebuild. The loop's cached PD state and physical actuators are not worth the complexity of live reconfigure.
- `Close(ctx)`: cancel loop context, `wg.Wait()` the goroutine.
- The vision-service dep is held as `vision.Service` (the interface), not the concrete `sunposition.Service` type. Any model implementing the vision API can substitute — useful for tests and for swapping in an ML detector later.

---

## 7. Module layout

```
sun-tracker/
├── go.mod                         # module: github.com/viam-devrel/sun-tracker
├── meta.json                      # both models declared
├── Makefile
├── README.md
├── cmd/
│   ├── module/main.go             # registers BOTH APIModel entries
│   └── cli/main.go                # smoke test (preserved)
├── sunposition/                   # vision service package
│   ├── config.go
│   ├── service.go                 # vision.Service impl
│   ├── quadrant.go                # YCbCr / Gray / generic dispatch
│   └── quadrant_test.go
└── tracker/                       # generic tracker service package
    ├── config.go
    ├── service.go                 # generic.Service impl + control loop
    ├── control.go                 # PD math, clamp, stepClamp (pure funcs)
    └── control_test.go
```

**Migration from the current scaffold:**

The current repo has `module.go` at the repo root in package `suntracker`, holding the vision-service stub. We:

1. Update `go.mod` from bare module name `suntracker` to `github.com/viam-devrel/sun-tracker`.
2. Move root `module.go` → `sunposition/service.go`. Rename package to `sunposition`. Rename the type `sunTrackerSunPosition` to something concise (e.g. `service`).
3. Update `cmd/module/main.go` and `cmd/cli/main.go` import paths.
4. Add `tracker/` package with stub registration.
5. Add the second model entry to `meta.json`.

`cmd/module/main.go`:

```go
package main

import (
    "github.com/viam-devrel/sun-tracker/sunposition"
    "github.com/viam-devrel/sun-tracker/tracker"

    "go.viam.com/rdk/module"
    "go.viam.com/rdk/resource"
    "go.viam.com/rdk/services/generic"
    "go.viam.com/rdk/services/vision"
)

func main() {
    module.ModularMain(
        resource.APIModel{API: vision.API,  Model: sunposition.Model},
        resource.APIModel{API: generic.API, Model: tracker.Model},
    )
}
```

The two packages are split deliberately so each has a narrow surface: the vision package never imports `servo`, the tracker package never imports `image`. They communicate only via the `vision.Service` interface.

---

## 8. Config schema (robot-level example)

```json
{
  "services": [
    {
      "name": "sun_vision",
      "type": "vision",
      "model": "devrel:sun-tracker:sun-position",
      "depends_on": ["my_camera"],
      "attributes": {
        "camera": "my_camera"
      }
    },
    {
      "name": "sun_tracker",
      "type": "generic",
      "model": "devrel:sun-tracker:sun-servo-tracker",
      "depends_on": ["sun_vision", "my_camera", "pan_servo", "tilt_servo"],
      "attributes": {
        "vision_service": "sun_vision",
        "camera":         "my_camera",
        "pan_servo":      "pan_servo",
        "tilt_servo":     "tilt_servo",

        "kp":             8.0,
        "kd":             0.0,
        "deadband":       0.05,
        "min_brightness": 30.0,

        "pan_sign":       1,
        "tilt_sign":      1,
        "pan_min":        0,
        "pan_max":        180,
        "tilt_min":       0,
        "tilt_max":       180,

        "loop_hz":        10,
        "max_step_degs":  5.0
      }
    }
  ]
}
```

`depends_on` for the tracker includes `my_camera` even though the tracker doesn't itself fetch images: this ensures startup ordering so the vision service's camera is alive before the tracker first calls it.

---

## 9. Error handling and edge cases

### 9.1 Startup
- Vision: missing camera dep → constructor returns error; module load fails fast with the dep name in the error.
- Tracker: missing vision / pan / tilt dep → same.
- Tracker stores the vision dep as `vision.Service`, never type-asserts to `sunposition.Service`. Any vision-API implementation is substitutable.

### 9.2 Per-tick (tracker loop)
- `DetectionsFromCamera` error → log Debug, skip tick, leave state untouched.
- Detection count ≠ 4 or unrecognized `ClassName` → log Warn (rate-limited dedupe), set `brightness = 0` → triggers the brightness-floor branch which freezes servos. Protects against a misconfigured vision service.
- `servo.Position` error → log Debug, skip that servo's write this tick.
- `servo.Move` error → log Debug, do not update the cached angle in state (treat servo as at previous position).

### 9.3 Brightness floor
If `brightness < MinBrightness`, update state with current `pan_err` / `tilt_err` for telemetry visibility, set `locked = false`, skip the servo write entirely. "No real sun in frame; do nothing."

### 9.4 Deadband
`|err| < Deadband` → step = 0 for that axis. If both steps are 0, `locked = true` and *both* `Move` calls are skipped (avoid re-issuing the same target every tick).

### 9.5 Context cancellation
All RDK calls (`cam.Images`, `servo.Position`, `servo.Move`, `visionSvc.DetectionsFromCamera`) use the loop context. `Close` cancels it; in-flight calls unwind; `wg.Wait` returns.

### 9.6 Concurrent state access
`state` struct (errors, angles, locked, enabled, brightness, last_update) protected by `sync.RWMutex`:
- **Writers**: the loop goroutine; `DoCommand({"enabled": ...})`; `DoCommand({"recenter": true})` indirectly via the angles it writes.
- **Readers**: `DoCommand({"get_state": true})`.

PD memory (`prev_err`, `prev_time`) lives only inside the loop goroutine — single writer, no lock needed.

### 9.7 DoCommand mid-tick
- `{"enabled": false}` arriving mid-tick: the in-flight tick completes (it read the flag at the start). Next tick sees disabled and skips. No mid-tick interruption.
- `{"recenter": true}` while enabled: races with the loop's next `Move`. Accepted; logged at Info — recenter is a debug verb, not meant to coexist with active tracking.

### 9.8 Explicitly not handled
- Camera reboot mid-loop (loop logs Debug errors until camera recovers — acceptable).
- Servo physical stalling (no closed-loop position feedback; we trust commands issued).
- Live re-tuning (config change → rebuild).

---

## 10. Testing strategy

### 10.1 `sunposition/quadrant_test.go` — pure image processing
- Synthetic `*image.YCbCr` with bright pixels in each quadrant → assert correct quadrant has highest score; signs/orientations correct.
- Uniform-brightness image → all four scores ≈ equal.
- `quadrantYCbCr` and `quadrantGeneric` agree on the same content within rounding tolerance — catches fast-path drift.
- `Rect.Min != (0,0)` (SubImage view) → strides handled correctly.
- Synthetic `*image.Gray` path covered similarly.

### 10.2 `sunposition/service_test.go` — vision service
- Stub `camera.Camera` returns a fixed `*image.YCbCr`. Wire via stub `resource.Dependencies`.
- `DetectionsFromCamera` → 4 detections, correct order (TL, TR, BL, BR), bounding boxes match image quadrants, scores reflect input.
- `GetProperties` returns `{DetectionSupported: true, ClassificationSupported: false, ObjectPCDsSupported: false}`.
- Unimplemented methods return the sentinel error.

### 10.3 `tracker/control_test.go` — pure PD math
- `|err| < Deadband` → step = 0.
- Outside deadband → signed step, clamped to `MaxStepDegs`.
- `dt == 0` first-call edge case → no NaN, no panic.
- `stepClamp` respects min/max bounds, rounds correctly, handles negative deltas.

### 10.4 `tracker/service_test.go` — control loop
- Stub `vision.Service` returning a configurable 4-detection set.
- Stub `servo.Servo` recording `Move` calls and reporting a settable `Position`.
- Drive `step(ctx, now)` directly (no ticker). Assert:
  - `brightness < MinBrightness` → no `Move` calls, `locked = false`, `enabled` stays true.
  - Bright in TR quadrant (`pan_sign=+1`) → pan moves right, tilt moves up.
  - Inside deadband → no Move calls, `locked = true`.
  - `pan_sign = -1` → servo direction inverts.
  - `DoCommand({"enabled": false})` → next `step` issues no Move.
  - `DoCommand({"get_state": true})` → all expected keys present, types correct.
  - `DoCommand({"recenter": true})` → both servos receive `Move(90, ...)`.
- Run with `-race`.

### 10.5 Out of scope for automated tests
- Real camera / real servos (covered by Phase 6 below, manually under booth conditions).
- Goroutine timing precision (`LoopHz` accuracy) — too flaky in CI; trust the ticker.

---

## 11. Tuning recipe

(Unchanged from `.plans/sun-tracker-design.md` §9 — preserved here for self-contained reading.)

Tune in this order:

1. **Static centering smoke test.** Use defaults. Aim a flashlight from one side. Watch `pan_error` in data manager; it should drive toward zero and stay there.
2. **Set `Kp`.** Watch for overshoot or oscillation. Defaults: `Kp=8 × max_err 0.7-0.9 = ~5-7°` per step, clipped by `MaxStepDegs=5`. The clip is desirable: bounds per-step travel time. Tune `Kp` up until oscillation, back off ~30%. Typical landing: 6-12.
3. **Add `Kd` if needed.** Backlash + one-frame image lag → overshoot. `Kd=0.5-1.5` damps it. Symptom: error crosses zero, undershoots back, settles. 2+ swings → raise `Kd`.
4. **`Deadband`.** Twitching at center → deadband too low. `0.05` ≈ a meaningful off-center light on 640-wide images. Drop to `0.03` if it locks too lazily.
5. **`MinBrightness`.** Tune on site. Point camera at a wall (no flashlight); read `brightness` from `get_state`. Set `MinBrightness` to ~2× that floor.
6. **Loop rate vs servo speed.** Max angular velocity = `MaxStepDegs × LoopHz`. `5° × 10Hz = 50°/sec` is comfortable for hobby servos. If `last_update` deltas stretch past `1/LoopHz`, the loop is bottlenecked on `Move()` — drop `LoopHz` to 5 or `MaxStepDegs` to 3.

---

## 12. Verification checklist (resolve before implementing)

Items that must be confirmed against the user's vendored RDK version. Do not assume the snippets here are correct on these points.

- [ ] **`NamedImage` accessor.** The exact way to extract an `image.Image` from `cam.Images(ctx, ...)` (`.Image()`, public `.Img`, or use `camera.DecodeImageFromCamera`) has shifted across RDK versions. Grep `~/go/pkg/mod/go.viam.com/rdk@<vers>/components/camera/`.
- [ ] **`DetectionsFromCamera` semantics.** Confirm how the `cameraName` parameter resolves against the dependency the vision service was constructed with — whether the service is expected to maintain its own camera cache keyed by name, or whether it should accept any camera by name from a registry.
- [ ] **Vision-service data-capture method names.** Which method gets captured by the data manager: `DetectionsFromCamera`, `CaptureAllFromCamera`, or both? Attribute schema differs.
- [ ] **`DoCommand` capture syntax.** The data-manager attribute key for passing a `command` payload through `DoCommand` capture (the `additional_params.command` shape in §6.4) — verify exact attribute name in the vendored RDK.
- [ ] **Servo `Move` blocking semantics.** Most underlying drivers can't read back actual servo arrival; `Move` returns when the command is set, not when the servo finishes traveling. If servos are slow (>100ms for 5°), the loop period stretches. Monitor `last_update` deltas in `get_state`.
- [ ] **Camera native format & exposure lock.** Run the module, check the startup log line in the vision service. If "falling back to slow path," investigate the camera's `format` / `mime_type` config. MJPEG → YCbCr; PNG/raw → RGBA or NRGBA. Lock exposure low in the camera component's config (key name varies — `exposure_auto`, `manual_exposure`, etc.) so only the "sun" saturates the sensor. Must happen before tuning gains.
- [ ] **Transform pipeline interaction.** A `resize` or other transform between the source camera and the vision service may re-encode to RGB and break the YCbCr fast path. Verify or avoid.
- [ ] **Servo sign convention.** Physical mount orientation determines `pan_sign` / `tilt_sign`. Find by experiment: shine light off-center, see which way the servo moves.
- [ ] **Module model namespace & meta.json.** Both models declared in `meta.json`; `devrel:sun-tracker` namespace consistent with `.viam-gen-info`.
- [ ] **Go module path.** `go.mod` reads `module github.com/viam-devrel/sun-tracker`. Existing `cmd/module/main.go` and `cmd/cli/main.go` use the bare `suntracker` import — update these.

---

## 13. Phased build plan

| Phase | Goal | Success criterion |
|---|---|---|
| 1 | **Scaffold reorg.** Update `go.mod` path. Move root `module.go` → `sunposition/service.go`. Add `tracker/` stub. Register both models in `cmd/module/main.go`. Add both to `meta.json`. | Module builds for `linux/arm64`; loads on a Pi with both models visible in the robot config UI. |
| 2 | **Vision: generic path.** Implement `Config`, `Validate`, constructor, `Close`. Implement `quadrantGeneric`, `quadrantError` dispatch, `DetectionsFromCamera`, `Detections`, `CaptureAllFromCamera`, `GetProperties`. Startup-log camera native format. | `DetectionsFromCamera` returns 4 detections from a real camera; quadrant scores change predictably when a flashlight moves across the frame. |
| 3 | **Vision: fast paths.** Implement `quadrantYCbCr` and `quadrantGray`. Add `quadrant_test.go` parity tests. | Startup log confirms YCbCr fast path engages on the real camera. Unit tests pass under `-race`. |
| 4 | **Tracker: loop scaffold.** Config, validate, constructor, `Close`, background goroutine ticking at `LoopHz`, calls vision service, logs scores. `DoCommand({"get_state": true})` returns state map (mostly zeros). No servo writes. | Module ticks at the configured rate; logs show vision-service responses; clean shutdown. |
| 5 | **Tracker: closed loop.** Implement `control` (PD + deadband + clamp), `stepClamp`, brightness-floor branch, servo drive. Implement `enabled` / `recenter` verbs. Add `tracker/control_test.go` and `tracker/service_test.go`. | Servos drive toward a flashlight, stop inside deadband, no oscillation with default gains. Unit tests pass under `-race`. |
| 6 | **Tuning + data manager.** Configure data-manager capture of `DoCommand{"get_state": true}` at 5Hz and `CaptureAllFromCamera` at 5Hz. Run the tuning recipe under booth lighting. Lock final gains in config. | Tracking smooth and visibly responsive. Telemetry visible in the Viam data UI. Locks within 1-2 seconds of the light being held still. |
| 7 | **Demo integration.** Compose with existing power sensor + dashboard (out of module scope). Unattended 30+ min booth run. | Clear power-output difference visible when tracking is enabled vs. disabled. |

---

## 14. Open questions / future work (out of scope here)

- **ML-based "sun" detector** as an alternative `vision.Service` model in the same module. Would slot in behind the same interface without tracker changes.
- **Multi-frame averaging** inside the vision service to suppress single-frame noise. Probably not needed at 10Hz with a reasonable exposure setting, but a known knob if booth lighting fights back.
- **Adaptive `MinBrightness`** that tracks ambient mean over a slow EMA. Avoids manual on-site tuning. Worth exploring only if the demo travels and lighting varies.
