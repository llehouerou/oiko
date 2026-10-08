# Research: telling a Recording worth a Notification from one the wind made (October 2026)

Question: Arlo's free plan does no image recognition, so every Recording of a
camera facing plants comes in as plain `motion`, wind included. What is a good
way to keep only the Recordings showing something notable (a person, a
vehicle, an animal)? Research only: nothing is decided or built here.

## What Oiko already has

- The cameras are Arlo Pro 2 (`VMC4030P`) on a `VMB4000` base. Arlo moved the
  Pro 2 to End of Life on 2025-01-01: no more firmware or feature updates, and
  "some features may not be available on EOL devices".
  [Arlo EOL](https://kb.arlo.com/000063018/End-of-Life-for-Arlo-Devices-and-Services)
- On the free plan the library gives `reason: motionRecord` and no
  `objCategory`: "no smart detection on this plan".
  [go-arlo NOTES, library](https://github.com/llehouerou/go-arlo/blob/master/docs/NOTES.md)
- go-arlo's `RecordingAdded` comes from the library notice sent **at the end of
  the upload**: the MP4 (1.2–1.3 MB for 18–20 s) and the 640×357 JPEG
  thumbnail can be read at once, about 6–14 s after the clip ends. So looking
  at the whole video adds no wait for it to be ready.
  [go-arlo NOTES, new recordings](https://github.com/llehouerou/go-arlo/blob/master/docs/NOTES.md)
- The `bridge` contract already carries a Recording's `Trigger` ("a short
  lower-case word (`motion`, `person`), or empty") as the data of the camera's
  `recording` Event, and an Event trigger fires on a set of values such as
  `person`. [ADR 0039](../adr/0039-a-new-recording-is-an-event-and-a-notification-may-carry-it.md),
  `bridge/bridge.go`
  → An Automation that sends a Notification only on `person` already exists as
  a concept. What's missing is something that sets that value.

## A. Fewer false triggers at the camera

- Arlo's own advice for fewer notifications is to re-aim the camera, lower the
  motion sensitivity, and review the mode's rules.
  [Arlo KB 1178858](https://kb.arlo.com/1178858/How-can-I-receive-fewer-Arlo-notifications)
  The Pro 2 manual has a Motion Detection Test (Device Utilities) to tune
  sensitivity per camera; its LED blinks amber on each detection.
  [Pro 2 manual](https://care.arlo.com/en-us/images/Documents/ArloPro2/arlo_pro_2_um.pdf), p. 12
- **Activity zones** (detect motion only in parts of the image) work on the
  Pro 2 **only while it is plugged into AC power**. A solar panel doesn't count.
  [Pro 2 manual](https://care.arlo.com/en-us/images/Documents/ArloPro2/arlo_pro_2_um.pdf), p. 52
  On battery the camera wakes on its passive infrared (PIR) sensor, which
  reacts to moving warm surfaces: sun-warmed leaves can trigger it (general
  PIR behaviour, not from an Arlo source). Zones would help only on a
  mains-powered camera whose plants sit outside the zone.
- **Arlo Secure** (paid) lists "Basic Person, Animal, Vehicle & Package
  Detection" and "Smart Activity Zones".
  [Arlo Secure plans](https://us.arlo.com/pages/arlo-secure)
  Its prices change by region and plan. Whether the EOL Pro 2 still gets smart
  detection isn't stated; check before paying. If it does, go-arlo would
  receive `objCategory` and oiko-arlo could pass it on as the Trigger.

Cheap and worth doing first, but on battery it only lowers the rate: it
doesn't sort a person from a plant.

## B. A local object detector on each Recording

Run a COCO-trained detector (80 classes, including person, car, dog, cat,
bird) on the Recording and turn "person found" into the Trigger.

**Frames to look at**
- The thumbnail alone needs no video decoding, but it's a single frame (which
  one Arlo picks isn't documented; to verify), and a person entering late may
  not be in it.
- Several frames of the MP4 (one per second, say) catch more, but Go has no
  H.264 decoder: `ffmpeg` is needed (nixpkgs `ffmpeg-headless` 9.0.1).

**Runtime**
- [ONNX Runtime](https://onnxruntime.ai) from Go:
  [`yalue/onnxruntime_go`](https://github.com/yalue/onnxruntime_go) (MIT,
  maintained, last push 2026-09). It **needs cgo** and the onnxruntime shared
  library at run time (`ort.SetSharedLibraryPath`). nixpkgs has
  `onnxruntime` 1.27.1. That makes the build non-pure-Go, a cost for Oiko or a
  Bridge built with `oiko-build`.
- A detector as a separate HTTP process instead:
  [DOODS2](https://github.com/snowzach/doods2) (MIT, last push 2026-06) has
  `POST /detect` taking an image and ships TFLite MobileNet-SSD, a TF Faster
  R-CNN and YOLOv5s. Python in Docker, which is one more service to run.
- Not suitable:
  - [Frigate](https://docs.frigate.video/configuration/object_detectors/) is an
    NVR that runs detection on its own continuous camera streams, not on clips
    handed to it. A battery Arlo has no such stream.
  - [CodeProject.AI Server](https://github.com/codeproject/CodeProject.AI-Server)
    has had no push since 2025-07.
  - OpenVINO is supported on 6th-gen Intel (Skylake) and newer, so an older
    Intel host's GPU path is out. ONNX Runtime runs on any x86-64 CPU with
    AVX2.
    [Frigate detectors](https://docs.frigate.video/configuration/object_detectors/)

**Models, and their licenses** (Oiko is public, so this matters if a model
ships with it)

| Model | License | ONNX available |
|---|---|---|
| [YOLOX](https://github.com/Megvii-BaseDetection/YOLOX) nano / tiny / s | Apache-2.0 | yes, in its [releases](https://github.com/Megvii-BaseDetection/YOLOX/releases/tag/0.1.1rc0) (repo idle since 2025-06) |
| [D-FINE](https://github.com/Peterande/D-FINE) | Apache-2.0 | export; supported by Frigate's ONNX detector |
| [RT-DETR](https://github.com/lyuwenyu/RT-DETR) | Apache-2.0 | export |
| [YOLOv9](https://github.com/WongKinYiu/yolov9) (Frigate's default) | GPL-3.0 | export |
| [Ultralytics YOLO](https://github.com/ultralytics/ultralytics) (v5/v8/11…) | AGPL-3.0, or a paid licence | export |

Frigate recommends 320×320 inputs and small variants. Its docs put around
30 ms of inference on a GPU as able to keep up with several cameras.
[Frigate detectors](https://docs.frigate.video/configuration/object_detectors/)
A few frames per Recording, a few dozen Recordings a day, is far below a
continuous NVR's load. Inference time on a home server's CPU isn't measured yet.

## C. A vision language model on each Recording

Send the thumbnail or a few frames to a multimodal model and ask "is there a
person, vehicle or animal?" (or for a caption). Home Assistant's
[LLM Vision](https://github.com/valentinfrlch/ha-llmvision) (Apache-2.0,
maintained) does this for camera events, with OpenAI, Anthropic, Gemini,
Ollama and OpenAI-compatible providers. It shows the pattern is established.

- **Cloud, Gemini Flash-Lite:** an image whose sides are both ≤ 384 px costs 258
  tokens, and a larger one is split into 768×768 tiles of 258 tokens each.
  [Gemini image understanding](https://ai.google.dev/gemini-api/docs/image-understanding)
  Input on the Flash-Lite models costs $0.10–$0.25 per million tokens.
  [Gemini pricing](https://ai.google.dev/gemini-api/docs/pricing)
  A thumbnail or a few frames therefore cost well under a cent per Recording.
  On the **free tier, content is used to improve Google's products**, which
  rules it out for pictures of a home: it has to be the paid tier. Every
  picture also leaves the house, and an API key and a network dependency
  appear.
- **Local, Ollama** (nixpkgs `ollama` 0.34.3) with a small vision model: no
  picture leaves the house, but a home server often has no usable GPU.
  CPU-only latency for a vision model is unmeasured here and likely seconds to
  tens of seconds per image, so the Notification would arrive late.
- An LLM may describe a plant as "movement" or hallucinate a person, while a
  detector's classes and confidence scores are fixed and testable. An LLM is
  better at captions ("a delivery driver at the gate") than at a yes/no filter.

## D. Pixel differences (frame diffing, masks)

Plants moving in the wind are real pixel motion, so diffing can't tell them
from a person without hand-drawn masks. Frigate itself uses motion only to
find regions it then hands to an object detector.
[Frigate detectors](https://docs.frigate.video/configuration/object_detectors/)
Not a solution on its own.

## Where it could fit in Oiko (options, not a decision)

- **In the Bridge** (oiko-arlo): classify before announcing, and report the
  Trigger as `person` / `vehicle` / `animal`, or `motion` when nothing is found.
  This needs no change to the `bridge` contract or to Automations (an Event
  trigger on `person`). But it serves Arlo only, and puts a model and its
  runtime in a Bridge.
- **In Oiko**, as something every camera's Recordings go through: it serves
  any camera system, but it's a new concept for `GLOSSARY.md` and probably an
  ADR. It may change what the `recording` Event's data means.
- Either way, a filter should drop the **Notification**, never the Recording,
  which stays in the camera's system and History. A missed person (a false
  negative) costs more than a plant let through.

## To measure before choosing

1. Which frame the Arlo thumbnail shows, by day and by night (IR, greyscale),
   for a person walking through.
2. Inference time on the target host's CPU for YOLOX-tiny/s or D-FINE-n at
   320/416/640, on the thumbnail and on about 5 frames.
3. Recall on real Recordings: a few dozen wind clips and a few dozen with
   people, kept off the repo.
4. Whether Arlo Secure still gives `objCategory` for a Pro 2, if paying is an
   option.
