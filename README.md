# sun-tracker

A Viam Go module for a solar-panel tracking demo. It drives a pan/tilt servo pair toward the brightest source in a camera's field of view — typically a handheld flashlight or motorized spotlight standing in for the sun. The module registers two cooperating services: `devrel:sun-tracker:sun-position` (vision) computes per-quadrant brightness from each camera frame, and `devrel:sun-tracker:sun-servo-tracker` (generic) runs the closed-loop PD controller that drives the servos.

---

## `devrel:sun-tracker:sun-position`

Vision service (`rdk:service:vision`). Returns four detections — one per image quadrant — with each `Score` set to that quadrant's mean luma (normalized to [0, 1] as a brightness proxy).

### Configuration

```json
{
  "camera": "my_camera"
}
```

The following attributes are available for the arm component:

| Name               | Type     | Inclusion    | Description                                                                                                                                                            |
| ------------------ | -------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `camera`             | string   | **Required** | The name of the camera component to use as the default camera for detections. |

### Implemented methods

| Method | Behavior |
|---|---|
| `DetectionsFromCamera` | Grabs an image from the configured camera, dispatches to quadrant computation, returns 4 detections. |
| `Detections` | Same computation on a caller-supplied `NamedImage`; bounding boxes derived from image bounds. |
| `CaptureAllFromCamera` | Returns the image plus 4 detections; used by data manager. |
| `GetProperties` | Returns `{DetectionSupported: true, ClassificationSupported: false, ObjectPCDsSupported: false}`. |
| `Classifications`, `ClassificationsFromCamera`, `GetObjectPointClouds` | Return unimplemented error. |

### Detection contract

Every call returns exactly 4 detections in fixed order:

| Index | ClassName / Label | Bounding box |
|---|---|---|
| 0 | `"top-left"` | top-left quadrant |
| 1 | `"top-right"` | top-right quadrant |
| 2 | `"bottom-left"` | bottom-left quadrant |
| 3 | `"bottom-right"` | bottom-right quadrant |

Bounding boxes split the image at the midpoint of each axis. Four detections are returned even in pitch black (all scores ~0). `Score` is mean luma / 255 — a brightness proxy, not a conventional detection confidence.

### Startup log

On first image capture the service logs the camera's native image type at `Info` level, for example:

```
camera native format: YCbCr (fast path)   subsample=YCbCrSubsampleRatio420  size=(640,480)
```

If the format is not YCbCr or Gray the service logs a warning and falls back to a slower per-pixel path. Check this log line to confirm the fast path is engaging; a transform pipeline (e.g. a resize step) can silently re-encode to RGB.

---

## `devrel:sun-tracker:sun-servo-tracker`

Generic service (`rdk:service:generic`). Polls a vision service at `loop_hz`, derives pan/tilt imbalance fractions and a brightness proxy from the 4 quadrant scores, applies PD control with deadband, and drives two servos.

### Configuration

```json
{
  "vision_service": "sun_vision",
  "camera":         "my_camera",
  "pan_servo":      "pan_servo",
  "tilt_servo":     "tilt_servo",
}
```
| Name               | Type     | Inclusion    | Description                                                                                                                                                            |
| ------------------ | -------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `camera`             | string   | **Required** | The name of the camera component to use as the camera for detections. |
| `vision_service`             | string   | **Required** | The name of the vision service to use for detections. |
| `pan_servo`             | string   | **Required** | The name of the servo component to use as the pan direction. |
| `tilt_servo`             | string   | **Required** | The name of the servo component to use as the tilt direction. |
| `kp`             | number   | Optional | The PID loop's proportional value. |
| `kd`             | number   | Optional | The PID loop's derivative value. |
| `deadband`             | number   | Optional | The percentage for significant change in movement. |
| `min_brightness`             | number   | Optional | The name of the camera component to use as the default camera for detections. |
| `pan_sign`             | number   | Optional | Flip the pan servo direction |
| `tilt_sign`             | number   | Optional | Flip the tilt servo direction |
| `pan_min`             | number   | Optional | The minimum angle for the pan servo |
| `pan_max`             | number   | Optional | The max angle for the pan servo. |
| `tilt_min`             | number   | Optional | The minimum angle for the tilt servo. |
| `tilt_max`             | number   | Optional | The max angle for the tilt servo. |
| `loop_hz`             | number   | Optional | The frequency of the control loop, number of times per second to check the vision service. |
| `max_step_degs`             | number   | Optional | The max step change for servo movement. |


`pan_sign` / `tilt_sign` (`1` or `-1`) flip servo direction without rewiring; determine the correct value by experiment (shine a light off-center, observe which way the servo moves).

### Defaults

| Field | Default |
|---|---|
| `kp` | 8.0 |
| `kd` | 0.0 |
| `deadband` | 0.05 |
| `min_brightness` | 30.0 |
| `pan_sign`, `tilt_sign` | 1 |
| `pan_min`, `tilt_min` | 0 |
| `pan_max`, `tilt_max` | 180 |
| `loop_hz` | 10 |
| `max_step_degs` | 5.0 |

### DoCommand

#### `get_state`

```json
{"get_state": true}
```

Returns a snapshot of the control-loop state:

```json
{
  "pan_error":   0.12,
  "tilt_error":  -0.04,
  "pan_deg":     94,
  "tilt_deg":    88,
  "brightness":  142.6,
  "locked":      false,
  "enabled":     true,
  "last_update": 1747080123456
}
```

`last_update` is Unix milliseconds. `locked` is true when both axes are inside the deadband and no servo moves were issued last tick.

#### `enabled`

```json
{"enabled": false}
```

Pause or resume the control loop. Returns `{"enabled": false}`. An in-flight tick completes; the next tick sees the updated flag.

```json
{"enabled": true}
```

#### `recenter`

```json
{"recenter": true}
```

Drives both servos to 90 degrees. Returns `{"recentered": true}`. Intended as a debug verb — it races with the control loop if tracking is enabled (logged at Info level).

### Recommended data capture

Add to the tracker service's resource config to record control state at 5 Hz:

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

---

## Behavior notes

The brightness floor (`min_brightness`, 0–255 scale) freezes both servos when mean luma is below the threshold. This prevents the tracker from chasing the brightest ambient patch when no real light source is in frame. Set `min_brightness` on-site: read `brightness` from `get_state` while pointing the camera at a plain wall with no flashlight, then set the threshold to roughly twice that floor value.

The deadband (`deadband`, fraction of total quadrant sum) suppresses servo movement when the imbalance is small. When both axes fall inside the deadband the loop sets `locked = true` and skips the `Move` calls entirely, avoiding continuous re-issuing of the same target. Both services use `resource.AlwaysRebuild` — a config change tears down and rebuilds the service, resetting PD memory and servo state. Live re-tuning is not supported; reload the config.
