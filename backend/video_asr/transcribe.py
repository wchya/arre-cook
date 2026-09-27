"""One bounded local transcription job. No listener, network client, or credentials.

Invoked by the Go video reader in a separate process group. The parent owns
admission, resource monitoring, cancellation, model verification, and cleanup.
"""
from __future__ import annotations

import argparse
import importlib.util
import json
import os
from pathlib import Path
import re
import resource
import signal
import subprocess
import sys
import time
import wave

MAX_MEDIA_BYTES = 20 << 20
MAX_SECONDS = 600
MAX_CHARS = 8000
SAMPLE_RATE = 16000
MAX_SPEECH_SECONDS = 6
TAG_RE = re.compile(r"<\|[^|]+\|>")


class JobError(Exception):
    pass


def enter_sandbox(job: Path, model: Path, ffmpeg: str) -> None:
    # -I excludes the script directory. Load only this root-owned sibling,
    # without enabling imports from the job directory or user site packages.
    spec = importlib.util.spec_from_file_location("video_sandbox", Path(__file__).with_name("sandbox.py"))
    sandbox = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(sandbox)
    try:
        sandbox.enter(job, model, Path(ffmpeg))
    except sandbox.SandboxUnavailable as error:
        raise JobError("sandbox_unavailable") from error


def restrict_decoder() -> None:
    resource.setrlimit(resource.RLIMIT_AS, (384 << 20, 384 << 20))
    resource.setrlimit(resource.RLIMIT_CPU, (15, 15))
    resource.setrlimit(resource.RLIMIT_FSIZE, (24 << 20, 24 << 20))


def decode_audio(source: Path, target: Path, ffmpeg: str) -> None:
    if source.is_symlink() or not source.is_file() or not 12 <= source.stat().st_size <= MAX_MEDIA_BYTES:
        raise JobError("invalid_media")
    try:
        subprocess.run(
            [ffmpeg, "-nostdin", "-hide_banner", "-loglevel", "error", "-y",
             "-protocol_whitelist", "file,pipe", "-format_whitelist", "mov,mp3,wav,flac,ogg,matroska,webm",
             "-max_alloc", "33554432", "-threads", "1", "-i", str(source),
             "-t", str(MAX_SECONDS + 1), "-map", "0:a:0", "-vn", "-sn", "-dn",
             "-ac", "1", "-ar", str(SAMPLE_RATE), "-c:a", "pcm_s16le", "-f", "wav", str(target)],
            stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
            timeout=18, check=True, preexec_fn=restrict_decoder,
        )
    except subprocess.TimeoutExpired as error:
        raise JobError("decode_timeout") from error
    except (OSError, subprocess.CalledProcessError) as error:
        raise JobError("invalid_media") from error


def inspect_wave(audio: wave.Wave_read) -> int:
    frames = audio.getnframes()
    if audio.getnchannels() != 1 or audio.getsampwidth() != 2 or audio.getframerate() != SAMPLE_RATE:
        raise JobError("invalid_media")
    if frames <= 0 or frames > MAX_SECONDS * SAMPLE_RATE:
        raise JobError("duration_limit")
    return frames


def speech_chunks(samples):
    # VAD's maximum speech duration is a hint and can be exceeded while it
    # searches for silence. Enforce the inference bound without losing samples.
    limit = MAX_SPEECH_SECONDS * SAMPLE_RATE
    for offset in range(0, len(samples), limit):
        yield offset, samples[offset:offset + limit]


def recognize(wav_path: Path, model_dir: Path) -> dict:
    # Import native libraries only after decoding. FFmpeg never inherits their
    # thread pools, and a failed/unsupported input never loads the ASR model.
    import numpy as np
    import sherpa_onnx

    with wave.open(str(wav_path), "rb") as audio:
        frames = inspect_wave(audio)
        recognizer = sherpa_onnx.OfflineRecognizer.from_sense_voice(
            model=str(model_dir / "model.int8.onnx"), tokens=str(model_dir / "tokens.txt"),
            num_threads=2, sample_rate=SAMPLE_RATE, provider="cpu", language="", use_itn=True, debug=False,
        )
        config = sherpa_onnx.VadModelConfig()
        config.silero_vad.model = str(model_dir / "silero_vad.onnx")
        config.silero_vad.min_silence_duration = 0.25
        config.silero_vad.min_speech_duration = 0.1
        config.silero_vad.max_speech_duration = MAX_SPEECH_SECONDS
        config.sample_rate = SAMPLE_RATE
        config.num_threads = 1
        config.provider = "cpu"
        vad = sherpa_onnx.VoiceActivityDetector(config, buffer_size_in_seconds=20)
        segments: list[dict] = []
        characters = 0

        def drain() -> None:
            nonlocal characters
            while not vad.empty():
                part = vad.front
                samples = part.samples
                if len(samples) > MAX_SECONDS * SAMPLE_RATE:
                    raise JobError("segment_limit")
                for offset, chunk in speech_chunks(samples):
                    stream = recognizer.create_stream()
                    stream.accept_waveform(SAMPLE_RATE, chunk)
                    recognizer.decode_stream(stream)
                    text = TAG_RE.sub("", stream.result.text or "").strip()
                    if text:
                        characters += len(text) + 1
                        if characters > MAX_CHARS or len(segments) >= 1000:
                            raise JobError("text_limit")
                        segments.append({"start": round((part.start + offset) / SAMPLE_RATE, 2),
                                         "end": round(min(frames, part.start + offset + len(chunk)) / SAMPLE_RATE, 2),
                                         "text": text})
                vad.pop()

        window = config.silero_vad.window_size
        while raw := audio.readframes(window):
            samples = np.frombuffer(raw, dtype="<i2").astype(np.float32) / 32768.0
            vad.accept_waveform(samples)
            drain()
        vad.flush()
        drain()
    if not segments:
        raise JobError("no_speech")
    text = "\n".join(s["text"] for s in segments)
    if len(text.encode("utf-8")) > 32 << 10:
        raise JobError("text_limit")
    return {"text": text, "duration_seconds": frames / SAMPLE_RATE, "segments": segments}


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", type=Path, required=True)
    parser.add_argument("--model-dir", type=Path, required=True)
    parser.add_argument("--ffmpeg", required=True)
    parser.add_argument("--check-runtime", action="store_true")
    args = parser.parse_args()
    os.umask(0o077)
    resource.setrlimit(resource.RLIMIT_CORE, (0, 0))
    # A backstop in addition to the parent's wall-time/RSS limits.
    resource.setrlimit(resource.RLIMIT_AS, (1536 << 20, 1536 << 20))
    signal.alarm(55)
    started = time.monotonic()
    wav_path = args.input.parent / "decoded.wav"
    try:
        enter_sandbox(args.input.parent, args.model_dir, args.ffmpeg)
        if args.check_runtime:
            # Verify native dependencies before advertising ASR availability.
            import numpy
            import sherpa_onnx
            if not numpy.__version__ or not callable(sherpa_onnx.OfflineRecognizer.from_sense_voice):
                raise JobError("runtime_error")
            print(json.dumps({"ready": True}), flush=True)
            return 0
        decode_audio(args.input, wav_path, args.ffmpeg)
        result = recognize(wav_path, args.model_dir)
        result["elapsed_seconds"] = round(time.monotonic() - started, 3)
        peak = resource.getrusage(resource.RUSAGE_SELF).ru_maxrss
        result["peak_rss_bytes"] = peak * 1024 if sys.platform != "darwin" else peak
        print(json.dumps(result, ensure_ascii=False), flush=True)
        return 0
    except JobError as error:
        print(json.dumps({"error": str(error)}), flush=True)
        return 2
    except Exception:
        # Native/runtime errors must not expose local paths, media, or model output.
        print(json.dumps({"error": "runtime_error"}), flush=True)
        return 2
    finally:
        wav_path.unlink(missing_ok=True)


if __name__ == "__main__":
    raise SystemExit(main())
