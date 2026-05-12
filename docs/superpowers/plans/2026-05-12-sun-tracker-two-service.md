# Sun Tracker Two-Service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement two cooperating Viam Go service models — `devrel:sun-tracker:sun-position` (vision service producing per-quadrant brightness detections) and `devrel:sun-tracker:sun-servo-tracker` (generic service that owns a closed PD control loop driving two servos) — replacing the current single-stub scaffold.

**Architecture:** Two service packages in one module. The vision service owns the camera dependency and computes per-quadrant luma with a YCbCr fast path; the generic tracker holds a vision-service dependency plus two servo dependencies and runs its control loop in a background goroutine. The two communicate only through the `vision.Service` interface; the tracker passes the configured camera name into `DetectionsFromCamera`.

**Tech Stack:** Go 1.21+, Viam RDK v0.125.0 (vendored at `~/go/pkg/mod/go.viam.com/rdk@v0.125.0`), standard library `image` package, `go.viam.com/test` (testify-style assertions).

**Spec:** `docs/superpowers/specs/2026-05-12-sun-tracker-two-service-design.md`

---

## Pre-flight: VERIFY items resolved against vendored RDK v0.125.0

These were marked **VERIFY** in the spec and have now been resolved by inspecting `~/go/pkg/mod/go.viam.com/rdk@v0.125.0`:

| Spec item | Resolution |
|---|---|
| `NamedImage` accessor | `func (ni *NamedImage) Image(ctx context.Context) (image.Image, error)` — takes a context, returns image + error. The original `.plans/sun-tracker-design.md` snippet `imgs[0].Image()` is wrong for v0.125.0. |
| `cam.Images` signature | `Images(ctx, filterSourceNames []string, extra map[string]interface{}) ([]NamedImage, resource.ResponseMetadata, error)` — call as `cam.Images(ctx, nil, nil)`. |
| `vision.FromDependencies` | `func FromDependencies(deps resource.Dependencies, name string) (Service, error)` — direct, no surprise. |
| `camera.FromDependencies` | `func FromDependencies(deps resource.Dependencies, name string) (Camera, error)`. |
| `generic.Service` registration | `resource.RegisterService(generic.API, model, resource.Registration[resource.Resource, *Config]{Constructor: ...})`. The "service type" parameter is just `resource.Resource`. |
| `resource.ErrDoUnimplemented` | Exists at `resource/resource.go:245`. |
| `resource.AlwaysRebuild` | Exists at `resource/resource.go:284` as an embeddable struct. |
| `vision.Properties` shape | `struct { ClassificationSupported, DetectionSupported, ObjectPCDsSupported bool }`. |

The remaining VERIFY items (data-manager `additional_params.command` syntax, transform-pipeline behavior, servo `Move` blocking semantics, camera exposure lock, servo sign convention) are runtime / config-time concerns that the tuning phase resolves on-device, not implementation-phase concerns. They are documented in the spec and the README; they do not block code tasks.

---

## File Structure

```
sun-tracker/
├── go.mod                              # MODIFY: module path → github.com/viam-devrel/sun-tracker
├── meta.json                           # MODIFY: declare both models
├── README.md                           # MODIFY: document both models
├── module.go                           # DELETE (moved to sunposition/service.go)
├── cmd/
│   ├── module/main.go                  # MODIFY: register both models, new import paths
│   └── cli/main.go                     # MODIFY: new import paths
├── sunposition/
│   ├── config.go                       # CREATE: Config + Validate
│   ├── service.go                      # CREATE: vision.Service impl, registration, startup format log
│   ├── service_test.go                 # CREATE: service-level tests (stub camera)
│   ├── quadrant.go                     # CREATE: quadrantError dispatch + 3 impls + buildDetections
│   └── quadrant_test.go                # CREATE: pure image-processing tests
└── tracker/
    ├── config.go                       # CREATE: Config + Validate + applyDefaults
    ├── service.go                      # CREATE: generic service, registration, runLoop, step, DoCommand
    ├── service_test.go                 # CREATE: control-loop tests (stub vision, stub servos)
    ├── control.go                      # CREATE: control(), clamp(), stepClamp() pure funcs
    └── control_test.go                 # CREATE: PD math tests
```

**Boundaries:**
- `sunposition` never imports `servo` or `tracker`.
- `tracker` never imports `image` or `sunposition` — it consumes the vision API as an interface.
- Pure-function math lives in `quadrant.go` and `control.go` so it's testable without RDK plumbing.

**Test stub strategy:** Each `_test.go` file declares the minimum stubs it needs (a stub `camera.Camera` for `sunposition/service_test.go`; a stub `vision.Service` and two stub `servo.Servo`s for `tracker/service_test.go`). Stubs live in the same package as the tests, in `_test.go` files (excluded from production builds).

---

## Conventions

**TDD discipline** — every task: write the failing test, run it to see it fail, implement, run it to see it pass, commit. Use @superpowers:test-driven-development.

**Verification** — every "run tests" step is also a checkpoint: if tests don't behave as the plan predicts, stop and investigate; don't paper over. Use @superpowers:verification-before-completion.

**Commit cadence** — one commit per task. Commit messages follow the existing repo style (`feat:`, `fix:`, `refactor:`, `docs:`, `test:`).

**Test runner** — `go test ./... -race` from repo root. Per-task tests can be narrowed with `go test ./sunposition -run TestName -v`.

**Always run** `go build ./cmd/module && go vet ./...` before each commit to catch silent breakage outside the modified package.

---

## Phase 1 — Scaffold reorganization

### Task 1.1: Update Go module path

**Files:**
- Modify: `go.mod`
- Modify: `cmd/module/main.go`
- Modify: `cmd/cli/main.go`

- [ ] **Step 1: Inspect current state**

```bash
head -3 go.mod
grep -n "suntracker" cmd/module/main.go cmd/cli/main.go
```

Expected: `go.mod` line 1 reads `module suntracker`; both `cmd/*/main.go` files import the bare `"suntracker"` path.

- [ ] **Step 2: Update `go.mod`**

Change line 1 from `module suntracker` to `module github.com/viam-devrel/sun-tracker`.

- [ ] **Step 3: Update `cmd/module/main.go` import**

Replace `"suntracker"` with `"github.com/viam-devrel/sun-tracker"`. Adjust the model reference accordingly (it'll still resolve via the package-level `SunPosition` symbol that currently lives in `package suntracker` at the repo root).

- [ ] **Step 4: Update `cmd/cli/main.go` import**

Same replacement.

- [ ] **Step 5: Verify build**

Run: `go build ./...`
Expected: builds cleanly. (The root `module.go` is still in `package suntracker` and still exports `SunPosition` — we move it in Task 1.2.)

- [ ] **Step 6: Commit**

```bash
git add go.mod cmd/module/main.go cmd/cli/main.go
git commit -m "refactor: set go module path to github.com/viam-devrel/sun-tracker"
```

---

### Task 1.2: Move root scaffold into `sunposition/` package

**Files:**
- Delete: `module.go`
- Create: `sunposition/service.go` (content reorganized from old `module.go`)

This task is structural: it relocates the existing stub without changing behavior. Vision-service logic comes in Phase 2.

- [ ] **Step 1: Create `sunposition/service.go`**

Move the content of root `module.go` into `sunposition/service.go` with:
- `package sunposition`
- Rename exported symbol `SunPosition` (the `resource.Model`) → keep it as `Model` (idiomatic — callers will refer to `sunposition.Model`)
- Rename type `sunTrackerSunPosition` → `service` (unexported, idiomatic; the type only escapes through the `vision.Service` interface)
- Rename `newSunTrackerSunPosition` → `newService`
- Drop the `NewSunPosition` factory that the CLI uses — replace with a thin helper of the same signature (we'll keep CLI smoke testing functional)
- Keep all `errUnimplemented` stub method returns as-is for this task

- [ ] **Step 2: Delete root `module.go`**

```bash
git rm module.go
```

- [ ] **Step 3: Update `cmd/module/main.go`**

```go
package main

import (
    "github.com/viam-devrel/sun-tracker/sunposition"

    "go.viam.com/rdk/module"
    "go.viam.com/rdk/resource"
    vision "go.viam.com/rdk/services/vision"
)

func main() {
    module.ModularMain(resource.APIModel{API: vision.API, Model: sunposition.Model})
}
```

- [ ] **Step 4: Update `cmd/cli/main.go`**

Replace the `suntracker.NewSunPosition(...)` call with `sunposition.NewService(...)` (whatever factory name was chosen in Step 1). Keep the smoke-test shape — context, deps, config — as before.

- [ ] **Step 5: Verify build**

Run: `go build ./...`
Expected: clean.

- [ ] **Step 6: Verify go vet**

Run: `go vet ./...`
Expected: no warnings.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor: move vision service scaffold into sunposition package"
```

---

### Task 1.3: Stub the `tracker` package

**Files:**
- Create: `tracker/config.go`
- Create: `tracker/service.go`

A minimal, registerable stub that does nothing — enough to load both models and verify routing. Real logic in Phases 4 and 5.

- [ ] **Step 1: Create `tracker/config.go`**

```go
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
```

- [ ] **Step 2: Create `tracker/service.go`**

```go
package tracker

import (
    "context"

    "go.viam.com/rdk/logging"
    "go.viam.com/rdk/resource"
    "go.viam.com/rdk/services/generic"
)

var Model = resource.NewModel("devrel", "sun-tracker", "sun-servo-tracker")

func init() {
    resource.RegisterService(generic.API, Model,
        resource.Registration[resource.Resource, *Config]{Constructor: newService},
    )
}

type service struct {
    resource.Named
    resource.AlwaysRebuild
    logger logging.Logger
}

func newService(
    ctx context.Context,
    deps resource.Dependencies,
    conf resource.Config,
    logger logging.Logger,
) (resource.Resource, error) {
    return &service{
        Named:  conf.ResourceName().AsNamed(),
        logger: logger,
    }, nil
}

func (s *service) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
    return nil, resource.ErrDoUnimplemented
}

func (s *service) Close(ctx context.Context) error { return nil }
```

- [ ] **Step 3: Update `cmd/module/main.go` to register both models**

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

- [ ] **Step 4: Verify build + vet**

Run: `go build ./... && go vet ./...`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: scaffold tracker package and register both models"
```

---

### Task 1.4: Update `meta.json` to declare both models

**Files:**
- Modify: `meta.json`

- [ ] **Step 1: Inspect current `meta.json`**

```bash
cat meta.json
```

Note that the current file lacks a `"models"` array — Viam's `module.schema.json` expects one. The existing description also says "Modular vision service: sun-position" which is now incomplete.

- [ ] **Step 2: Add `models` array and update `description`**

Update `meta.json` so it includes:
```json
{
  "$schema": "https://dl.viam.dev/module.schema.json",
  "module_id": "devrel:sun-tracker",
  "visibility": "public",
  "url": "",
  "description": "Sun tracker: brightness-quadrant vision service + servo pan/tilt tracker",
  "applications": null,
  "markdown_link": "README.md",
  "models": [
    {
      "api": "rdk:service:vision",
      "model": "devrel:sun-tracker:sun-position",
      "markdown_link": "README.md"
    },
    {
      "api": "rdk:service:generic",
      "model": "devrel:sun-tracker:sun-servo-tracker",
      "markdown_link": "README.md"
    }
  ],
  "entrypoint": "bin/sun-tracker",
  "first_run": "",
  "build": {
    "build": "make module.tar.gz",
    "setup": "make setup",
    "path": "module.tar.gz",
    "arch": ["linux/amd64", "linux/arm64", "darwin/arm64", "windows/amd64"]
  }
}
```

- [ ] **Step 3: Verify build still passes**

Run: `go build ./...`
Expected: clean (meta.json doesn't affect build, but no other regressions).

- [ ] **Step 4: Commit**

```bash
git add meta.json
git commit -m "chore: declare both models in meta.json"
```

---

## Phase 2 — Vision service: generic image-processing path

### Task 2.1: Implement `sunposition.Config`

**Files:**
- Modify: `sunposition/config.go` (extract from `service.go`)
- Modify: `sunposition/service.go`

- [ ] **Step 1: Create `sunposition/config.go`**

```go
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
```

- [ ] **Step 2: Remove the empty `Config` struct + `Validate` from `service.go`**

Leave the rest of `service.go` (registration, type, stub methods) unchanged for this task.

- [ ] **Step 3: Verify build**

Run: `go build ./... && go vet ./...`
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "feat(sunposition): require camera in Config.Validate"
```

---

### Task 2.2: Implement `quadrantGeneric` and write tests

**Files:**
- Create: `sunposition/quadrant.go`
- Create: `sunposition/quadrant_test.go`

- [ ] **Step 1: Write failing test for `quadrantGeneric` with one bright quadrant**

`sunposition/quadrant_test.go`:
```go
package sunposition

import (
    "image"
    "image/color"
    "testing"

    "go.viam.com/test"
)

// fillRect paints a rectangle on an RGBA image with a flat color.
func fillRect(img *image.RGBA, r image.Rectangle, c color.RGBA) {
    for y := r.Min.Y; y < r.Max.Y; y++ {
        for x := r.Min.X; x < r.Max.X; x++ {
            img.SetRGBA(x, y, c)
        }
    }
}

func TestQuadrantGeneric_TopRightBright(t *testing.T) {
    img := image.NewRGBA(image.Rect(0, 0, 100, 100))
    // Fill the top-right quadrant with white; rest is black (zero value).
    fillRect(img, image.Rect(50, 0, 100, 50), color.RGBA{255, 255, 255, 255})

    tl, tr, bl, br, brightness := quadrantGeneric(img)

    test.That(t, tr, test.ShouldBeGreaterThan, tl)
    test.That(t, tr, test.ShouldBeGreaterThan, bl)
    test.That(t, tr, test.ShouldBeGreaterThan, br)
    // brightness is mean luma 0-1 (we'll divide by 255 in the caller).
    test.That(t, brightness, test.ShouldBeGreaterThan, 0.0)
}
```

- [ ] **Step 2: Run test — expect failure (function not defined)**

Run: `go test ./sunposition -run TestQuadrantGeneric_TopRightBright -v`
Expected: FAIL — undefined: quadrantGeneric.

- [ ] **Step 3: Implement `quadrantGeneric`**

`sunposition/quadrant.go`:
```go
package sunposition

import "image"

// quadrantGeneric returns mean-luma per quadrant (TL, TR, BL, BR) and overall
// mean luma, each normalized to [0, 1] (255 → 1.0). Slow path: uses At().RGBA()
// per pixel — used when the source image isn't *image.YCbCr or *image.Gray.
func quadrantGeneric(img image.Image) (tl, tr, bl, br, brightness float64) {
    b := img.Bounds()
    midX := (b.Min.X + b.Max.X) / 2
    midY := (b.Min.Y + b.Max.Y) / 2

    var sums [4]float64 // 0=TL, 1=TR, 2=BL, 3=BR
    var counts [4]int

    for y := b.Min.Y; y < b.Max.Y; y++ {
        for x := b.Min.X; x < b.Max.X; x++ {
            r, g, bl, _ := img.At(x, y).RGBA()
            // RGBA returns 16-bit values; shift to 8-bit then weight.
            luma := 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(bl>>8)
            i := 0
            if y >= midY {
                i += 2
            }
            if x >= midX {
                i += 1
            }
            sums[i] += luma
            counts[i]++
        }
    }
    norm := func(s float64, n int) float64 {
        if n == 0 {
            return 0
        }
        return (s / float64(n)) / 255.0
    }
    tl = norm(sums[0], counts[0])
    tr = norm(sums[1], counts[1])
    bl = norm(sums[2], counts[2])
    br = norm(sums[3], counts[3])
    total := sums[0] + sums[1] + sums[2] + sums[3]
    totalCount := counts[0] + counts[1] + counts[2] + counts[3]
    if totalCount > 0 {
        brightness = (total / float64(totalCount)) / 255.0
    }
    return
}
```

- [ ] **Step 4: Run test — expect pass**

Run: `go test ./sunposition -run TestQuadrantGeneric_TopRightBright -v`
Expected: PASS.

- [ ] **Step 5: Add coverage tests for each quadrant + uniform image**

Append to `quadrant_test.go`:
```go
func TestQuadrantGeneric_AllFourQuadrants(t *testing.T) {
    cases := []struct {
        name    string
        rect    image.Rectangle
        winner  string // "tl"|"tr"|"bl"|"br"
    }{
        {"TL", image.Rect(0, 0, 50, 50), "tl"},
        {"TR", image.Rect(50, 0, 100, 50), "tr"},
        {"BL", image.Rect(0, 50, 50, 100), "bl"},
        {"BR", image.Rect(50, 50, 100, 100), "br"},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            img := image.NewRGBA(image.Rect(0, 0, 100, 100))
            fillRect(img, tc.rect, color.RGBA{255, 255, 255, 255})
            tl, tr, bl, br, _ := quadrantGeneric(img)
            scores := map[string]float64{"tl": tl, "tr": tr, "bl": bl, "br": br}
            winner := scores[tc.winner]
            for k, v := range scores {
                if k == tc.winner {
                    continue
                }
                test.That(t, winner, test.ShouldBeGreaterThan, v)
            }
        })
    }
}

func TestQuadrantGeneric_UniformImage(t *testing.T) {
    img := image.NewRGBA(image.Rect(0, 0, 100, 100))
    fillRect(img, img.Bounds(), color.RGBA{128, 128, 128, 255})
    tl, tr, bl, br, brightness := quadrantGeneric(img)
    // All four near-equal.
    test.That(t, tl-tr, test.ShouldAlmostEqual, 0.0, 0.01)
    test.That(t, tl-bl, test.ShouldAlmostEqual, 0.0, 0.01)
    test.That(t, tl-br, test.ShouldAlmostEqual, 0.0, 0.01)
    // 128/255 ≈ 0.50.
    test.That(t, brightness, test.ShouldAlmostEqual, 0.5, 0.01)
}
```

- [ ] **Step 6: Run all sunposition tests**

Run: `go test ./sunposition -race -v`
Expected: 3 tests pass.

- [ ] **Step 7: Commit**

```bash
git add sunposition/quadrant.go sunposition/quadrant_test.go
git commit -m "feat(sunposition): add quadrantGeneric image-processing slow path"
```

---

### Task 2.3: Add `buildDetections` and `quadrantError` dispatch

**Files:**
- Modify: `sunposition/quadrant.go`
- Modify: `sunposition/quadrant_test.go`

- [ ] **Step 1: Write failing test for `buildDetections`**

Append to `quadrant_test.go`:
```go
import (
    objdet "go.viam.com/rdk/vision/objectdetection"
)

func TestBuildDetections_FourInFixedOrder(t *testing.T) {
    bounds := image.Rect(0, 0, 200, 100)
    dets := buildDetections(bounds, 0.10, 0.80, 0.30, 0.50)

    test.That(t, len(dets), test.ShouldEqual, 4)

    expected := []struct {
        label string
        score float64
        rect  image.Rectangle
    }{
        {"top-left",     0.10, image.Rect(0, 0, 100, 50)},
        {"top-right",    0.80, image.Rect(100, 0, 200, 50)},
        {"bottom-left",  0.30, image.Rect(0, 50, 100, 100)},
        {"bottom-right", 0.50, image.Rect(100, 50, 200, 100)},
    }
    for i, want := range expected {
        test.That(t, dets[i].Label(), test.ShouldEqual, want.label)
        test.That(t, dets[i].Score(), test.ShouldAlmostEqual, want.score, 1e-9)
        bb := dets[i].BoundingBox()
        test.That(t, *bb, test.ShouldResemble, want.rect)
    }
    _ = objdet.Detection(nil) // silence unused import if dropped
}
```

- [ ] **Step 2: Run test — expect failure**

Run: `go test ./sunposition -run TestBuildDetections -v`
Expected: FAIL — undefined: buildDetections.

- [ ] **Step 3: Implement `buildDetections` + `quadrantError` dispatch**

Append to `quadrant.go`:
```go
import (
    objdet "go.viam.com/rdk/vision/objectdetection"
)

// quadrantError dispatches to a type-specific implementation. Returns mean-luma
// per quadrant (TL, TR, BL, BR) and overall mean — all normalized to [0,1].
func quadrantError(img image.Image) (tl, tr, bl, br, brightness float64) {
    switch t := img.(type) {
    // YCbCr and Gray fast paths added in Task 3.x.
    default:
        _ = t
        return quadrantGeneric(img)
    }
}

// buildDetections returns four detections in fixed order TL, TR, BL, BR with
// stable labels. Quadrant rectangles split the bounds at the midpoint.
func buildDetections(bounds image.Rectangle, tl, tr, bl, br float64) []objdet.Detection {
    midX := (bounds.Min.X + bounds.Max.X) / 2
    midY := (bounds.Min.Y + bounds.Max.Y) / 2

    quads := []struct {
        label string
        rect  image.Rectangle
        score float64
    }{
        {"top-left",     image.Rect(bounds.Min.X, bounds.Min.Y, midX, midY),           tl},
        {"top-right",    image.Rect(midX,         bounds.Min.Y, bounds.Max.X, midY),    tr},
        {"bottom-left",  image.Rect(bounds.Min.X, midY,         midX, bounds.Max.Y),    bl},
        {"bottom-right", image.Rect(midX,         midY,         bounds.Max.X, bounds.Max.Y), br},
    }
    out := make([]objdet.Detection, 0, 4)
    for _, q := range quads {
        rect := q.rect
        out = append(out, objdet.NewDetection(bounds, rect, q.score, q.label))
    }
    return out
}
```

Note: `objdet.NewDetection` signature is `(imgBounds, bbox, score, label)` — verify by inspecting `~/go/pkg/mod/go.viam.com/rdk@v0.125.0/vision/objectdetection/detection.go` if the test fails on signature mismatch.

- [ ] **Step 4: Run test — expect pass**

Run: `go test ./sunposition -race -v`
Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add sunposition/quadrant.go sunposition/quadrant_test.go
git commit -m "feat(sunposition): add buildDetections and quadrantError dispatch"
```

---

### Task 2.4: Wire vision service to grab camera frames and return detections

**Files:**
- Modify: `sunposition/service.go`
- Create: `sunposition/service_test.go`

This is the most "Viam-shaped" task in Phase 2. It changes the service from a registered-but-empty stub into a working detector.

- [ ] **Step 1: Write failing service-level test with stub camera**

`sunposition/service_test.go`:
```go
package sunposition

import (
    "context"
    "image"
    "image/color"
    "testing"

    "go.viam.com/rdk/components/camera"
    "go.viam.com/rdk/logging"
    "go.viam.com/rdk/resource"
    "go.viam.com/test"
)

// stubCamera implements just enough of camera.Camera for DetectionsFromCamera tests.
type stubCamera struct {
    camera.Camera
    resource.Named
    img image.Image
}

func newStubCamera(name string, img image.Image) *stubCamera {
    return &stubCamera{
        Named: resource.NewName(camera.API, name).AsNamed(),
        img:   img,
    }
}

func (c *stubCamera) Images(
    ctx context.Context,
    filter []string,
    extra map[string]interface{},
) ([]camera.NamedImage, resource.ResponseMetadata, error) {
    ni, err := camera.NamedImageFromImage(c.img, "", "", nil)
    if err != nil {
        return nil, resource.ResponseMetadata{}, err
    }
    return []camera.NamedImage{*ni}, resource.ResponseMetadata{}, nil
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

    conf := resource.Config{
        Name:       "sun_vision",
        API:        camera.API, // overridden below by registration helper
        Model:      Model,
        Attributes: nil,
    }
    cfg := &Config{Camera: "my_camera"}

    svc, err := newServiceWithConfig(context.Background(), deps, conf, cfg, logger)
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
```

- [ ] **Step 2: Run test — expect failure**

Run: `go test ./sunposition -run TestDetectionsFromCamera_FourInFixedOrder -v`
Expected: FAIL — undefined: newServiceWithConfig, or no detections returned.

- [ ] **Step 3: Implement service constructor + `DetectionsFromCamera` + `Detections` + `CaptureAllFromCamera` + `GetProperties`**

Rewrite `sunposition/service.go`:
```go
package sunposition

import (
    "context"
    "errors"
    "fmt"
    "image"
    "sync"

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
    return newServiceWithConfig(ctx, deps, conf, cfg, logger)
}

// newServiceWithConfig is the test-friendly entrypoint: caller has already
// converted to a typed *Config.
func newServiceWithConfig(
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
func (s *service) logFormatOnce(img image.Image) {
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
    s.logFormatOnce(img)
    return s.detectionsForImage(img), nil
}

func (s *service) Detections(
    ctx context.Context,
    img image.Image,
    extra map[string]interface{},
) ([]objdet.Detection, error) {
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
    s.logFormatOnce(img)
    out := viscapture.VisCapture{}
    if opts.ReturnImage {
        out.Image = img
    }
    if opts.ReturnDetections {
        out.Detections = s.detectionsForImage(img)
    }
    return out, nil
}

func (s *service) Classifications(
    ctx context.Context,
    img image.Image,
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
```

Note: this rewrite replaces the old stub in `service.go`. The shape uses the `vision.Service` interface accessor methods (`Detection.Label()`, `.Score()`, `.BoundingBox()`) — check `~/go/pkg/mod/go.viam.com/rdk@v0.125.0/vision/objectdetection/detection.go` for exact accessor names if the test won't compile.

- [ ] **Step 4: Update `cmd/cli/main.go` to use `newServiceWithConfig`**

The CLI was already pointing at a `NewSunPosition` factory; this is now `newServiceWithConfig`. Adjust the import + call. Empty `Camera` field will fail validation, but the CLI just needs to compile and exit cleanly — set `Camera: "stub"` and accept that it'll error at runtime; the CLI is a smoke test, not a real exerciser.

- [ ] **Step 5: Run service test + build**

Run: `go test ./sunposition -race -v && go build ./...`
Expected: tests pass, build clean.

- [ ] **Step 6: Add `GetProperties` test**

Append to `service_test.go`:
```go
func TestGetProperties_DetectionOnly(t *testing.T) {
    logger := logging.NewTestLogger(t)
    cam := newStubCamera("my_camera", newRGBAWithBrightTR())
    deps := resource.Dependencies{cam.Name(): cam}
    conf := resource.Config{Name: "sun_vision", Model: Model}
    cfg := &Config{Camera: "my_camera"}

    svc, err := newServiceWithConfig(context.Background(), deps, conf, cfg, logger)
    test.That(t, err, test.ShouldBeNil)

    props, err := svc.GetProperties(context.Background(), nil)
    test.That(t, err, test.ShouldBeNil)
    test.That(t, props.DetectionSupported, test.ShouldBeTrue)
    test.That(t, props.ClassificationSupported, test.ShouldBeFalse)
    test.That(t, props.ObjectPCDsSupported, test.ShouldBeFalse)
}
```

Run: `go test ./sunposition -race -v`
Expected: 5 tests pass.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat(sunposition): grab frames and return per-quadrant detections"
```

---

## Phase 3 — Vision service: YCbCr and Gray fast paths

### Task 3.1: Implement `quadrantYCbCr`

**Files:**
- Modify: `sunposition/quadrant.go`
- Modify: `sunposition/quadrant_test.go`

- [ ] **Step 1: Write failing parity test**

Append to `quadrant_test.go`:
```go
import (
    "image/draw"
)

// ycbcrFromRGBA converts an RGBA test image to a *image.YCbCr at full chroma
// resolution so test inputs match exactly.
func ycbcrFromRGBA(t *testing.T, src *image.RGBA) *image.YCbCr {
    t.Helper()
    ycbcr := image.NewYCbCr(src.Bounds(), image.YCbCrSubsampleRatio444)
    // Manually convert pixel-by-pixel using the same matrix Go uses internally.
    for y := src.Bounds().Min.Y; y < src.Bounds().Max.Y; y++ {
        for x := src.Bounds().Min.X; x < src.Bounds().Max.X; x++ {
            r, g, b, _ := src.At(x, y).RGBA()
            yy, cb, cr := rgbToYCbCr(uint8(r>>8), uint8(g>>8), uint8(b>>8))
            yi := ycbcr.YOffset(x, y)
            ci := ycbcr.COffset(x, y)
            ycbcr.Y[yi] = yy
            ycbcr.Cb[ci] = cb
            ycbcr.Cr[ci] = cr
        }
    }
    _ = draw.Draw // silence unused if draw becomes unused later
    return ycbcr
}

// Same conversion the stdlib uses (image/color/ycbcr.go).
func rgbToYCbCr(r, g, b uint8) (uint8, uint8, uint8) {
    fr := float64(r)
    fg := float64(g)
    fb := float64(b)
    y := 0.299*fr + 0.587*fg + 0.114*fb
    cb := -0.168736*fr - 0.331264*fg + 0.5*fb + 128
    cr := 0.5*fr - 0.418688*fg - 0.081312*fb + 128
    clip := func(v float64) uint8 {
        switch {
        case v < 0:
            return 0
        case v > 255:
            return 255
        default:
            return uint8(v)
        }
    }
    return clip(y), clip(cb), clip(cr)
}

func TestQuadrantYCbCr_ParityWithGeneric(t *testing.T) {
    rgba := newRGBAWithBrightTR()
    ycbcr := ycbcrFromRGBA(t, rgba)

    gtl, gtr, gbl, gbr, gbright := quadrantGeneric(rgba)
    ytl, ytr, ybl, ybr, ybright := quadrantYCbCr(ycbcr)

    // Conversion + integer rounding can introduce small differences. 0.01 = 2.5/255 luma drift max.
    test.That(t, ytl, test.ShouldAlmostEqual, gtl, 0.01)
    test.That(t, ytr, test.ShouldAlmostEqual, gtr, 0.01)
    test.That(t, ybl, test.ShouldAlmostEqual, gbl, 0.01)
    test.That(t, ybr, test.ShouldAlmostEqual, gbr, 0.01)
    test.That(t, ybright, test.ShouldAlmostEqual, gbright, 0.01)
}

func TestQuadrantYCbCr_SubImageView(t *testing.T) {
    full := image.NewYCbCr(image.Rect(0, 0, 200, 200), image.YCbCrSubsampleRatio444)
    // Fill the whole image with mid-gray luma.
    for i := range full.Y {
        full.Y[i] = 128
    }
    sub := full.SubImage(image.Rect(100, 100, 200, 200)).(*image.YCbCr)
    tl, tr, bl, br, brightness := quadrantYCbCr(sub)
    test.That(t, tl, test.ShouldAlmostEqual, 128.0/255.0, 0.01)
    test.That(t, tr, test.ShouldAlmostEqual, 128.0/255.0, 0.01)
    test.That(t, bl, test.ShouldAlmostEqual, 128.0/255.0, 0.01)
    test.That(t, br, test.ShouldAlmostEqual, 128.0/255.0, 0.01)
    test.That(t, brightness, test.ShouldAlmostEqual, 128.0/255.0, 0.01)
}
```

- [ ] **Step 2: Run test — expect failure**

Run: `go test ./sunposition -run TestQuadrantYCbCr -v`
Expected: FAIL — undefined: quadrantYCbCr.

- [ ] **Step 3: Implement `quadrantYCbCr`**

Append to `quadrant.go`:
```go
// quadrantYCbCr is the fast path. Indexes the Y plane directly — no interface
// dispatch, no allocation, no per-pixel matrix math. Subtracts Rect.Min so
// SubImage views work correctly.
func quadrantYCbCr(img *image.YCbCr) (tl, tr, bl, br, brightness float64) {
    b := img.Rect
    midX := (b.Min.X + b.Max.X) / 2
    midY := (b.Min.Y + b.Max.Y) / 2

    var sums [4]uint64
    var counts [4]int

    for y := b.Min.Y; y < b.Max.Y; y++ {
        rowStart := (y - b.Min.Y) * img.YStride
        for x := b.Min.X; x < b.Max.X; x++ {
            luma := uint64(img.Y[rowStart+(x-b.Min.X)])
            i := 0
            if y >= midY {
                i += 2
            }
            if x >= midX {
                i += 1
            }
            sums[i] += luma
            counts[i]++
        }
    }
    norm := func(s uint64, n int) float64 {
        if n == 0 {
            return 0
        }
        return (float64(s) / float64(n)) / 255.0
    }
    tl = norm(sums[0], counts[0])
    tr = norm(sums[1], counts[1])
    bl = norm(sums[2], counts[2])
    br = norm(sums[3], counts[3])
    total := sums[0] + sums[1] + sums[2] + sums[3]
    totalCount := counts[0] + counts[1] + counts[2] + counts[3]
    if totalCount > 0 {
        brightness = (float64(total) / float64(totalCount)) / 255.0
    }
    return
}
```

- [ ] **Step 4: Add `*image.YCbCr` case to `quadrantError` dispatch**

```go
func quadrantError(img image.Image) (tl, tr, bl, br, brightness float64) {
    switch t := img.(type) {
    case *image.YCbCr:
        return quadrantYCbCr(t)
    default:
        return quadrantGeneric(img)
    }
}
```

- [ ] **Step 5: Run tests — expect pass**

Run: `go test ./sunposition -race -v`
Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add sunposition/quadrant.go sunposition/quadrant_test.go
git commit -m "feat(sunposition): add YCbCr fast path for quadrant computation"
```

---

### Task 3.2: Implement `quadrantGray`

**Files:**
- Modify: `sunposition/quadrant.go`
- Modify: `sunposition/quadrant_test.go`

- [ ] **Step 1: Write failing test**

Append to `quadrant_test.go`:
```go
func TestQuadrantGray_BrightBottomLeft(t *testing.T) {
    img := image.NewGray(image.Rect(0, 0, 100, 100))
    for y := 50; y < 100; y++ {
        for x := 0; x < 50; x++ {
            img.SetGray(x, y, color.Gray{255})
        }
    }
    _, _, bl, _, _ := quadrantGray(img)
    tl, tr, _, br, _ := quadrantGray(img)
    test.That(t, bl, test.ShouldBeGreaterThan, tl)
    test.That(t, bl, test.ShouldBeGreaterThan, tr)
    test.That(t, bl, test.ShouldBeGreaterThan, br)
}
```

- [ ] **Step 2: Run test — expect failure**

Run: `go test ./sunposition -run TestQuadrantGray -v`
Expected: FAIL — undefined: quadrantGray.

- [ ] **Step 3: Implement and wire dispatch**

Append to `quadrant.go`:
```go
func quadrantGray(img *image.Gray) (tl, tr, bl, br, brightness float64) {
    b := img.Rect
    midX := (b.Min.X + b.Max.X) / 2
    midY := (b.Min.Y + b.Max.Y) / 2

    var sums [4]uint64
    var counts [4]int

    for y := b.Min.Y; y < b.Max.Y; y++ {
        rowStart := (y - b.Min.Y) * img.Stride
        for x := b.Min.X; x < b.Max.X; x++ {
            luma := uint64(img.Pix[rowStart+(x-b.Min.X)])
            i := 0
            if y >= midY {
                i += 2
            }
            if x >= midX {
                i += 1
            }
            sums[i] += luma
            counts[i]++
        }
    }
    norm := func(s uint64, n int) float64 {
        if n == 0 {
            return 0
        }
        return (float64(s) / float64(n)) / 255.0
    }
    tl = norm(sums[0], counts[0])
    tr = norm(sums[1], counts[1])
    bl = norm(sums[2], counts[2])
    br = norm(sums[3], counts[3])
    total := sums[0] + sums[1] + sums[2] + sums[3]
    totalCount := counts[0] + counts[1] + counts[2] + counts[3]
    if totalCount > 0 {
        brightness = (float64(total) / float64(totalCount)) / 255.0
    }
    return
}
```

Update `quadrantError`:
```go
func quadrantError(img image.Image) (tl, tr, bl, br, brightness float64) {
    switch t := img.(type) {
    case *image.YCbCr:
        return quadrantYCbCr(t)
    case *image.Gray:
        return quadrantGray(t)
    default:
        return quadrantGeneric(img)
    }
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./sunposition -race -v`
Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add sunposition/quadrant.go sunposition/quadrant_test.go
git commit -m "feat(sunposition): add Gray fast path for quadrant computation"
```

---

## Phase 4 — Tracker service: control-loop scaffold

### Task 4.1: Implement `tracker.Config` with defaults

**Files:**
- Modify: `tracker/config.go`
- Create: `tracker/config_test.go`

- [ ] **Step 1: Write failing tests for defaults**

`tracker/config_test.go`:
```go
package tracker

import (
    "testing"

    "go.viam.com/test"
)

func TestApplyDefaults_FillsZerosOnly(t *testing.T) {
    c := &Config{
        VisionService: "v", Camera: "c", PanServo: "p", TiltServo: "t",
    }
    applyDefaults(c)
    test.That(t, c.Kp, test.ShouldEqual, 8.0)
    test.That(t, c.Deadband, test.ShouldEqual, 0.05)
    test.That(t, c.MinBrightness, test.ShouldEqual, 30.0)
    test.That(t, c.PanSign, test.ShouldEqual, 1)
    test.That(t, c.TiltSign, test.ShouldEqual, 1)
    test.That(t, c.PanMax, test.ShouldEqual, uint32(180))
    test.That(t, c.TiltMax, test.ShouldEqual, uint32(180))
    test.That(t, c.LoopHz, test.ShouldEqual, 10.0)
    test.That(t, c.MaxStepDegs, test.ShouldEqual, 5.0)
}

func TestApplyDefaults_PreservesNonZero(t *testing.T) {
    c := &Config{
        VisionService: "v", Camera: "c", PanServo: "p", TiltServo: "t",
        Kp: 3.0, PanSign: -1, LoopHz: 20.0,
    }
    applyDefaults(c)
    test.That(t, c.Kp, test.ShouldEqual, 3.0)
    test.That(t, c.PanSign, test.ShouldEqual, -1)
    test.That(t, c.LoopHz, test.ShouldEqual, 20.0)
}

func TestValidate_RequiresDeps(t *testing.T) {
    c := &Config{}
    _, _, err := c.Validate("services.0")
    test.That(t, err, test.ShouldNotBeNil)

    c = &Config{VisionService: "v", Camera: "c", PanServo: "p", TiltServo: "t"}
    required, _, err := c.Validate("services.0")
    test.That(t, err, test.ShouldBeNil)
    test.That(t, required, test.ShouldResemble, []string{"v", "c", "p", "t"})
}
```

- [ ] **Step 2: Run test — expect failure**

Run: `go test ./tracker -run TestApplyDefaults -v`
Expected: FAIL — undefined: applyDefaults.

- [ ] **Step 3: Extend `config.go`**

Replace the contents of `tracker/config.go`:
```go
package tracker

import "errors"

type Config struct {
    VisionService string `json:"vision_service"`
    Camera        string `json:"camera"`
    PanServo      string `json:"pan_servo"`
    TiltServo     string `json:"tilt_servo"`

    Kp            float64 `json:"kp,omitempty"`
    Kd            float64 `json:"kd,omitempty"`
    Deadband      float64 `json:"deadband,omitempty"`
    MinBrightness float64 `json:"min_brightness,omitempty"` // 0-255 mean luma

    PanSign       int    `json:"pan_sign,omitempty"`
    TiltSign      int    `json:"tilt_sign,omitempty"`
    PanMin        uint32 `json:"pan_min,omitempty"`
    PanMax        uint32 `json:"pan_max,omitempty"`
    TiltMin       uint32 `json:"tilt_min,omitempty"`
    TiltMax       uint32 `json:"tilt_max,omitempty"`

    LoopHz       float64 `json:"loop_hz,omitempty"`
    MaxStepDegs  float64 `json:"max_step_degs,omitempty"`
}

func (c *Config) Validate(path string) ([]string, []string, error) {
    if c.VisionService == "" || c.Camera == "" || c.PanServo == "" || c.TiltServo == "" {
        return nil, nil, errors.New(path + ": vision_service, camera, pan_servo, tilt_servo are required")
    }
    return []string{c.VisionService, c.Camera, c.PanServo, c.TiltServo}, nil, nil
}

func applyDefaults(c *Config) {
    if c.Kp == 0 {
        c.Kp = 8.0
    }
    if c.Deadband == 0 {
        c.Deadband = 0.05
    }
    if c.MinBrightness == 0 {
        c.MinBrightness = 30.0
    }
    if c.PanSign == 0 {
        c.PanSign = 1
    }
    if c.TiltSign == 0 {
        c.TiltSign = 1
    }
    if c.PanMax == 0 {
        c.PanMax = 180
    }
    if c.TiltMax == 0 {
        c.TiltMax = 180
    }
    if c.LoopHz == 0 {
        c.LoopHz = 10
    }
    if c.MaxStepDegs == 0 {
        c.MaxStepDegs = 5.0
    }
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./tracker -race -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add tracker/config.go tracker/config_test.go
git commit -m "feat(tracker): full Config with defaults and validation"
```

---

### Task 4.2: Add PD control math with tests

**Files:**
- Create: `tracker/control.go`
- Create: `tracker/control_test.go`

- [ ] **Step 1: Write failing tests for `control`, `clamp`, `stepClamp`**

`tracker/control_test.go`:
```go
package tracker

import (
    "math"
    "testing"

    "go.viam.com/test"
)

func TestControl_DeadbandReturnsZero(t *testing.T) {
    step := control(0.03, 0.0, 0.1, 8.0, 0.0, 0.05, 5.0)
    test.That(t, step, test.ShouldEqual, 0.0)
}

func TestControl_OutsideDeadbandReturnsClampedStep(t *testing.T) {
    step := control(0.5, 0.0, 0.1, 8.0, 0.0, 0.05, 5.0)
    // Raw = 8 * 0.5 = 4.0 — under the clamp.
    test.That(t, step, test.ShouldAlmostEqual, 4.0, 1e-9)
}

func TestControl_ClampedToMaxStep(t *testing.T) {
    step := control(1.0, 0.0, 0.1, 20.0, 0.0, 0.05, 5.0)
    // Raw = 20.0 — clipped to 5.0.
    test.That(t, step, test.ShouldEqual, 5.0)
}

func TestControl_NegativeError(t *testing.T) {
    step := control(-0.8, 0.0, 0.1, 8.0, 0.0, 0.05, 5.0)
    test.That(t, step, test.ShouldEqual, -5.0) // clamped on the negative side too
}

func TestControl_DtZeroDoesNotPanic(t *testing.T) {
    step := control(0.5, 0.0, 0.0, 8.0, 2.0, 0.05, 5.0)
    test.That(t, math.IsNaN(step), test.ShouldBeFalse)
    // dt=0 → derivative term skipped → proportional only.
    test.That(t, step, test.ShouldAlmostEqual, 4.0, 1e-9)
}

func TestClamp(t *testing.T) {
    test.That(t, clamp(0.0, -1.0, 1.0), test.ShouldEqual, 0.0)
    test.That(t, clamp(-2.0, -1.0, 1.0), test.ShouldEqual, -1.0)
    test.That(t, clamp(2.0, -1.0, 1.0), test.ShouldEqual, 1.0)
}

func TestStepClamp(t *testing.T) {
    test.That(t, stepClamp(90, 5.0, 0, 180), test.ShouldEqual, uint32(95))
    test.That(t, stepClamp(90, -5.4, 0, 180), test.ShouldEqual, uint32(85))
    test.That(t, stepClamp(178, 5.0, 0, 180), test.ShouldEqual, uint32(180))
    test.That(t, stepClamp(2, -5.0, 0, 180), test.ShouldEqual, uint32(0))
}
```

- [ ] **Step 2: Run test — expect failure**

Run: `go test ./tracker -run TestControl -v`
Expected: FAIL — undefined identifiers.

- [ ] **Step 3: Implement `control.go`**

```go
package tracker

import "math"

// control returns a step in degrees (signed). Returns 0 inside the deadband.
// dt=0 (first call) skips the derivative term.
func control(err, prevErr, dt, kp, kd, deadband, maxStep float64) float64 {
    if math.Abs(err) < deadband {
        return 0
    }
    step := kp * err
    if dt > 0 {
        step += kd * (err - prevErr) / dt
    }
    return clamp(step, -maxStep, maxStep)
}

func clamp(v, lo, hi float64) float64 {
    switch {
    case v < lo:
        return lo
    case v > hi:
        return hi
    default:
        return v
    }
}

// stepClamp applies a signed angular delta to the current angle, then clamps
// to the [lo, hi] range. The delta is rounded to the nearest integer degree
// since servo positions are uint32.
func stepClamp(current uint32, delta float64, lo, hi uint32) uint32 {
    target := int(current) + int(math.Round(delta))
    if target < int(lo) {
        target = int(lo)
    }
    if target > int(hi) {
        target = int(hi)
    }
    return uint32(target)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./tracker -race -v`
Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add tracker/control.go tracker/control_test.go
git commit -m "feat(tracker): add PD control math with deadband and clamp"
```

---

### Task 4.3: Add error parsing — `detectionsToErrors`

**Files:**
- Modify: `tracker/control.go` (or new file)
- Modify: `tracker/control_test.go`

Helper that takes a 4-detection slice and produces (pan_err, tilt_err, brightness_0to255).

- [ ] **Step 1: Write failing test**

Append to `control_test.go`:
```go
import (
    "image"
    objdet "go.viam.com/rdk/vision/objectdetection"
)

func mkDet(label string, score float64) objdet.Detection {
    bounds := image.Rect(0, 0, 100, 100)
    return objdet.NewDetection(bounds, image.Rect(0, 0, 50, 50), score, label)
}

func TestDetectionsToErrors_BrightInTR(t *testing.T) {
    dets := []objdet.Detection{
        mkDet("top-left",     0.10),
        mkDet("top-right",    0.80),
        mkDet("bottom-left",  0.10),
        mkDet("bottom-right", 0.10),
    }
    panErr, tiltErr, brightness, ok := detectionsToErrors(dets)
    test.That(t, ok, test.ShouldBeTrue)
    // total = 1.10. panErr = (0.8+0.1 - 0.1-0.1)/1.10 = 0.7/1.10 ≈ 0.636
    // tiltErr = (0.10+0.80 - 0.10-0.10)/1.10 = 0.7/1.10 ≈ 0.636 (positive = up)
    test.That(t, panErr, test.ShouldBeGreaterThan, 0.5)
    test.That(t, tiltErr, test.ShouldBeGreaterThan, 0.5)
    // brightness = mean * 255 = (1.10/4)*255 ≈ 70
    test.That(t, brightness, test.ShouldAlmostEqual, 70.125, 0.1)
}

func TestDetectionsToErrors_WrongCountReturnsNotOK(t *testing.T) {
    _, _, _, ok := detectionsToErrors(nil)
    test.That(t, ok, test.ShouldBeFalse)
    _, _, _, ok = detectionsToErrors([]objdet.Detection{mkDet("top-left", 0.5)})
    test.That(t, ok, test.ShouldBeFalse)
}

func TestDetectionsToErrors_UnknownLabelReturnsNotOK(t *testing.T) {
    dets := []objdet.Detection{
        mkDet("top-left",  0.10),
        mkDet("top-right", 0.80),
        mkDet("xyz",       0.10), // bad label
        mkDet("bottom-right", 0.10),
    }
    _, _, _, ok := detectionsToErrors(dets)
    test.That(t, ok, test.ShouldBeFalse)
}

func TestDetectionsToErrors_AllZerosReturnsZeroErr(t *testing.T) {
    dets := []objdet.Detection{
        mkDet("top-left",     0.0),
        mkDet("top-right",    0.0),
        mkDet("bottom-left",  0.0),
        mkDet("bottom-right", 0.0),
    }
    panErr, tiltErr, brightness, ok := detectionsToErrors(dets)
    test.That(t, ok, test.ShouldBeTrue)
    test.That(t, panErr, test.ShouldEqual, 0.0)
    test.That(t, tiltErr, test.ShouldEqual, 0.0)
    test.That(t, brightness, test.ShouldEqual, 0.0)
}
```

- [ ] **Step 2: Run — expect failure**

Run: `go test ./tracker -run TestDetectionsToErrors -v`
Expected: FAIL — undefined: detectionsToErrors.

- [ ] **Step 3: Implement `detectionsToErrors`**

Append to `tracker/control.go`:
```go
import (
    objdet "go.viam.com/rdk/vision/objectdetection"
)

// detectionsToErrors converts the 4-detection vision output into pan/tilt
// imbalance fractions and a 0-255 brightness proxy. Returns ok=false if the
// detection set is malformed (wrong count or unrecognized label).
func detectionsToErrors(dets []objdet.Detection) (panErr, tiltErr, brightness float64, ok bool) {
    if len(dets) != 4 {
        return 0, 0, 0, false
    }
    var tl, tr, bl, br float64
    var seen [4]bool
    for _, d := range dets {
        switch d.Label() {
        case "top-left":
            tl = d.Score()
            seen[0] = true
        case "top-right":
            tr = d.Score()
            seen[1] = true
        case "bottom-left":
            bl = d.Score()
            seen[2] = true
        case "bottom-right":
            br = d.Score()
            seen[3] = true
        default:
            return 0, 0, 0, false
        }
    }
    for _, s := range seen {
        if !s {
            return 0, 0, 0, false
        }
    }
    total := tl + tr + bl + br
    brightness = (total / 4.0) * 255.0
    if total == 0 {
        return 0, 0, brightness, true
    }
    panErr = (tr + br - tl - bl) / total
    tiltErr = (tl + tr - bl - br) / total
    return panErr, tiltErr, brightness, true
}
```

Note: the `import` block above is illustrative — merge into the existing imports in `control.go`.

- [ ] **Step 4: Run tests**

Run: `go test ./tracker -race -v`
Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add tracker/control.go tracker/control_test.go
git commit -m "feat(tracker): parse 4-detection set into pan/tilt errors"
```

---

### Task 4.4: Wire tracker constructor to resolve deps

**Files:**
- Modify: `tracker/service.go`

- [ ] **Step 1: Update `service.go` to resolve vision + servo deps in `newService`**

Replace the body of `newService` and the `service` struct fields:
```go
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
    pan, err := servo.FromDependencies(deps, cfg.PanServo)
    if err != nil {
        return nil, errors.Join(errors.New("resolving pan servo"), err)
    }
    tilt, err := servo.FromDependencies(deps, cfg.TiltServo)
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

// runLoop is implemented in Task 4.5.
func (s *service) runLoop(ctx context.Context) { defer s.wg.Done() }

func (s *service) Close(ctx context.Context) error {
    s.cancel()
    s.wg.Wait()
    return nil
}

func (s *service) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
    return nil, resource.ErrDoUnimplemented
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./... && go vet ./...`
Expected: clean (the runLoop is a no-op, will be replaced next).

- [ ] **Step 3: Commit**

```bash
git add tracker/service.go
git commit -m "refactor(tracker): resolve vision + servo deps in constructor"
```

---

### Task 4.5: Implement loop scaffold with `step` extraction (no servo writes yet)

**Files:**
- Modify: `tracker/service.go`
- Create: `tracker/service_test.go`

The loop runs a ticker, but for now `step` only calls the vision service and updates state (no `Move` calls). This lets us test the goroutine plumbing in isolation.

- [ ] **Step 1: Write failing test using stubs**

`tracker/service_test.go`:
```go
package tracker

import (
    "context"
    "image"
    "sync"
    "testing"
    "time"

    "go.viam.com/rdk/components/servo"
    "go.viam.com/rdk/logging"
    "go.viam.com/rdk/resource"
    vision "go.viam.com/rdk/services/vision"
    objdet "go.viam.com/rdk/vision/objectdetection"
    "go.viam.com/test"
)

// stubVision returns a configurable detection set on every DetectionsFromCamera call.
type stubVision struct {
    resource.Named
    resource.TriviallyCloseable
    resource.TriviallyReconfigurable

    mu       sync.Mutex
    dets     []objdet.Detection
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

// All other vision.Service methods unused — return empties.
func (v *stubVision) Detections(ctx context.Context, img image.Image, extra map[string]interface{}) ([]objdet.Detection, error) {
    return nil, nil
}
func (v *stubVision) ClassificationsFromCamera(context.Context, string, int, map[string]interface{}) (classifications, error) { return nil, nil }
func (v *stubVision) Classifications(context.Context, image.Image, int, map[string]interface{}) (classifications, error) { return nil, nil }
func (v *stubVision) GetObjectPointClouds(context.Context, string, map[string]interface{}) (objects, error) { return nil, nil }
func (v *stubVision) GetProperties(context.Context, map[string]interface{}) (*vision.Properties, error) { return &vision.Properties{}, nil }
func (v *stubVision) CaptureAllFromCamera(context.Context, string, viscapture, map[string]interface{}) (visCapture, error) { return visCapture{}, nil }
func (v *stubVision) DoCommand(context.Context, map[string]interface{}) (map[string]interface{}, error) { return nil, nil }

// The aliased types above are placeholders — replace with the real types from
// go.viam.com/rdk/vision/classification, go.viam.com/rdk/vision (vis.Object),
// and go.viam.com/rdk/vision/viscapture. See sunposition/service.go for the
// canonical method signatures.

// stubServo records every Move call and reports a settable position.
type stubServo struct {
    resource.Named
    resource.TriviallyCloseable
    resource.TriviallyReconfigurable

    mu     sync.Mutex
    pos    uint32
    moves  []uint32
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

func (s *stubServo) Stop(ctx context.Context, extra map[string]interface{}) error    { return nil }
func (s *stubServo) IsMoving(ctx context.Context) (bool, error)                       { return false, nil }
func (s *stubServo) DoCommand(context.Context, map[string]interface{}) (map[string]interface{}, error) { return nil, nil }

func (s *stubServo) moveCount() int {
    s.mu.Lock()
    defer s.mu.Unlock()
    return len(s.moves)
}

// buildTestService wires stubs into a *service for direct step() / DoCommand
// testing. It does NOT start the goroutine — tests drive step() directly.
func buildTestService(t *testing.T, vis *stubVision, pan, tilt *stubServo) *service {
    t.Helper()
    cfg := &Config{
        VisionService: "v", Camera: "c", PanServo: "p", TiltServo: "t",
    }
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
    return []objdet.Detection{
        mkDet("top-left",     0.05),
        mkDet("top-right",    0.80),
        mkDet("bottom-left",  0.05),
        mkDet("bottom-right", 0.10),
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
    test.That(t, pe, test.ShouldBeGreaterThan, 0.0) // light is right → positive pan_err
}
```

Note on the stub: the method signatures in the snippet above use placeholder types. When compiling, replace with the real `classification.Classifications`, `[]*vis.Object`, `viscapture.CaptureOptions`, `viscapture.VisCapture` from the relevant RDK packages. Look at `sunposition/service.go` for the canonical method signatures the stub needs to implement.

- [ ] **Step 2: Run test — expect failure**

Run: `go test ./tracker -run TestStep_BrightInTopRight -v`
Expected: FAIL — undefined: step, or stub interface unimplemented.

- [ ] **Step 3: Implement `step` (with servo writes already wired)**

Replace the empty `runLoop` and add `step` in `tracker/service.go`:
```go
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

// lastPan / lastTilt return cached angles under lock (used by tryReadServo
// when a Position read fails).
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
```

Add the loop-local state fields to the `service` struct (still protected by `s.mu`, but only the loop goroutine and `step` write them — DoCommand should not):
```go
type service struct {
    // ...existing fields...

    // PD memory — written only inside step()
    lastPanErr  float64
    lastTiltErr float64
    lastTime    time.Time
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./tracker -race -v`
Expected: PASS — `TestStep_BrightInTopRight_MovesPanRight` should now pass with pan moves recorded and `state.PanErr > 0`.

- [ ] **Step 5: Commit**

```bash
git add tracker/service.go tracker/service_test.go
git commit -m "feat(tracker): closed-loop step with PD, deadband, and servo drive"
```

---

## Phase 5 — Tracker service: DoCommand surface + edge-case tests

### Task 5.1: Implement `DoCommand` verbs

**Files:**
- Modify: `tracker/service.go`
- Modify: `tracker/service_test.go`

- [ ] **Step 1: Write failing tests**

Append to `tracker/service_test.go`:
```go
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
    // Pan error positive (light in TR)
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
```

- [ ] **Step 2: Run — expect failures**

Run: `go test ./tracker -race -v`
Expected: FAILs on the new tests (`get_state` returns ErrDoUnimplemented today).

- [ ] **Step 3: Replace stub `DoCommand` with real implementation**

In `tracker/service.go`:
```go
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
```

- [ ] **Step 4: Run tests**

Run: `go test ./tracker -race -v`
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add tracker/service.go tracker/service_test.go
git commit -m "feat(tracker): DoCommand verbs get_state/enabled/recenter"
```

---

### Task 5.2: Edge-case tests — brightness floor, deadband, sign flip

**Files:**
- Modify: `tracker/service_test.go`

- [ ] **Step 1: Write the three remaining edge-case tests**

Append to `service_test.go`:
```go
func TestStep_BelowBrightnessFloor_NoMove(t *testing.T) {
    vis := newStubVision("v")
    pan := newStubServo("p", 90)
    tilt := newStubServo("t", 90)
    // All scores small → brightness < default MinBrightness (30/255 ≈ 0.118)
    vis.setDets([]objdet.Detection{
        mkDet("top-left",     0.01),
        mkDet("top-right",    0.02),
        mkDet("bottom-left",  0.01),
        mkDet("bottom-right", 0.01),
    })
    s := buildTestService(t, vis, pan, tilt)

    s.step(context.Background(), time.Now())

    test.That(t, pan.moveCount(), test.ShouldEqual, 0)
    test.That(t, tilt.moveCount(), test.ShouldEqual, 0)
    s.mu.RLock()
    locked := s.state.Locked
    s.mu.RUnlock()
    test.That(t, locked, test.ShouldBeFalse)
}

func TestStep_InsideDeadband_LockedNoMove(t *testing.T) {
    vis := newStubVision("v")
    pan := newStubServo("p", 90)
    tilt := newStubServo("t", 90)
    // Slight imbalance, well inside deadband.
    vis.setDets([]objdet.Detection{
        mkDet("top-left",     0.50),
        mkDet("top-right",    0.51),
        mkDet("bottom-left",  0.50),
        mkDet("bottom-right", 0.50),
    })
    s := buildTestService(t, vis, pan, tilt)

    s.step(context.Background(), time.Now())

    test.That(t, pan.moveCount(), test.ShouldEqual, 0)
    test.That(t, tilt.moveCount(), test.ShouldEqual, 0)
    s.mu.RLock()
    locked := s.state.Locked
    s.mu.RUnlock()
    test.That(t, locked, test.ShouldBeTrue)
}

func TestStep_PanSignFlip_InvertsDirection(t *testing.T) {
    vis := newStubVision("v")
    pan := newStubServo("p", 90)
    tilt := newStubServo("t", 90)
    vis.setDets(brightTRDets(t))
    s := buildTestService(t, vis, pan, tilt)
    s.cfg.PanSign = -1 // flip pan

    s.step(context.Background(), time.Now())

    // pan_err is positive (light right); with PanSign=-1, servo should move LEFT (lower angle).
    pos, _ := pan.Position(context.Background(), nil)
    test.That(t, pos, test.ShouldBeLessThan, uint32(90))
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./tracker -race -v`
Expected: all pass (the prior implementation already handles these — these tests just lock in the spec'd behavior).

- [ ] **Step 3: Commit**

```bash
git add tracker/service_test.go
git commit -m "test(tracker): brightness floor, deadband-lock, and sign-flip cases"
```

---

## Phase 6 — Documentation

### Task 6.1: Rewrite `README.md` to cover both models

**Files:**
- Modify: `README.md`
- Modify: `devrel_sun-tracker_sun-position.md` (existing placeholder — update or delete)

- [ ] **Step 1: Write a focused README**

Rewrite `README.md` to:
- Briefly describe the module purpose (sun-tracking demo for solar panels)
- Document `devrel:sun-tracker:sun-position` — config example, what it produces, `DetectionsFromCamera` / `CaptureAllFromCamera`, the 4-detection contract and stable labels
- Document `devrel:sun-tracker:sun-servo-tracker` — config example with all attributes + defaults table, the three DoCommand verbs, the recommended data-manager capture config
- Link to the spec at `docs/superpowers/specs/2026-05-12-sun-tracker-two-service-design.md` for design rationale
- Link to the tuning recipe (§11 of the spec) for on-device tuning

Keep the README terse — config + DoCommand examples + one-paragraph behavior summary per model. Avoid restating the spec verbatim.

- [ ] **Step 2: Decide on `devrel_sun-tracker_sun-position.md`**

If the README now covers both models cleanly, delete the per-model placeholder file. If you keep it, update its content to match reality (it currently has generator-template `attribute_1`/`attribute_2` placeholders).

- [ ] **Step 3: Verify everything still builds + tests pass**

Run: `go build ./... && go test ./... -race`
Expected: clean, all tests pass.

- [ ] **Step 4: Commit**

```bash
git add README.md devrel_sun-tracker_sun-position.md
git commit -m "docs: describe both module models with config + DoCommand examples"
```

---

## Final verification

After Task 6.1, run the full project checks before declaring the implementation complete. Use @superpowers:verification-before-completion.

- [ ] **Build for both target architectures**

```bash
GOOS=darwin GOARCH=arm64 go build -o /tmp/sun-tracker-darwin ./cmd/module
GOOS=linux  GOARCH=arm64 go build -o /tmp/sun-tracker-linux ./cmd/module
```

Both succeed → cross-compile is healthy.

- [ ] **Full test suite under -race**

```bash
go test ./... -race -count=1
```

All packages pass.

- [ ] **`go vet`**

```bash
go vet ./...
```

No warnings.

- [ ] **`make module.tar.gz`** (if `make setup` already ran)

```bash
make module.tar.gz
```

Tarball produced (final shape: `bin/sun-tracker` + meta.json + README + supporting files).

- [ ] **Manual smoke test (optional, requires a Pi)**

Out of scope for this plan — covered by Phase 6/7 of the spec's phased build plan (tuning + demo integration).

---

## What's deliberately not in this plan

- **On-device tuning** (spec §11). Recipe is in the spec; running it requires hardware.
- **Power-sensor integration / dashboard** (spec §2 non-goal).
- **ML-based sun detector** (spec §14 future work). The vision-service interface boundary leaves the door open without changing the tracker.
- **Live re-tuning via DoCommand**. `AlwaysRebuild` + a config edit is the supported re-tune path.
- **Goroutine timing assertions on `LoopHz`**. Too flaky for CI. The ticker is trusted.

---

## Skills referenced

- @superpowers:test-driven-development — every task body
- @superpowers:verification-before-completion — final verification section
- @superpowers:subagent-driven-development or @superpowers:executing-plans — for execution
