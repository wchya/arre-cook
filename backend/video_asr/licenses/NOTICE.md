# Local video transcription

The recipe application's local worker is independent of the referenced workbench;
it does not install or run a separate web application.

- **SenseVoiceSmall**: FunAudioLLM / Alibaba Group. The model retains its original
  name and is used under the [FunASR model license](SenseVoice-MODEL-LICENSE.txt),
  not the workbench's MIT license. Original model:
  <https://huggingface.co/FunAudioLLM/SenseVoiceSmall>.
  The quantized ONNX conversion is distributed by csukuangfj / k2-fsa:
  <https://huggingface.co/csukuangfj/sherpa-onnx-sense-voice-zh-en-ja-ko-yue-2024-07-17>.
  The exact revision, sizes, and SHA-256 digests are in `../model.lock.json` in the
  application image and `backend/internal/video/model.lock.json` in the repository.
- **Silero VAD**: Silero Team, MIT; see [the retained license](Silero-VAD-MIT.txt).
  Source: <https://github.com/snakers4/silero-vad>. The pinned ONNX artifact is
  distributed by <https://github.com/k2-fsa/sherpa-onnx>.
- **sherpa-onnx**: k2-fsa contributors, Apache-2.0. Source:
  <https://github.com/k2-fsa/sherpa-onnx>. The installed wheel includes its license.
- **NumPy**: NumPy Developers, BSD-3-Clause; bundled numerical libraries have
  additional notices included in the installed wheel.
- **video-transcript-workbench**: splexuan, MIT. Its Bilibili subtitle discovery,
  SenseVoice runtime, and Douyin guest-credential / metadata / smallest-media
  workflow informed this integration. The server uses its existing guarded Go
  HTTP reader, without installing the workbench, yt-dlp, or a browser. See the retained
  [MIT notice](video-transcript-workbench-MIT.txt) and source:
  <https://github.com/splexuan/video-transcript-workbench>.
- **FFmpeg / libseccomp**: Debian-packaged binaries and libraries; their package
  copyright and license notices remain under `/usr/share/doc` in the image.

Code dependencies are pinned with wheel hashes in `requirements.txt`; model
weights are verified separately and mounted read-only. The worker neither
downloads code nor installs models while processing a video.
