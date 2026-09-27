import io
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch
import wave

import transcribe
import download_models


class TranscriptionBoundaryTests(unittest.TestCase):
    def test_long_vad_segments_are_bounded_without_losing_audio(self):
        limit = transcribe.MAX_SPEECH_SECONDS * transcribe.SAMPLE_RATE
        samples = range(limit * 3 + 37)
        cursor = 0
        for offset, chunk in transcribe.speech_chunks(samples):
            self.assertEqual(offset, cursor)
            self.assertGreater(len(chunk), 0)
            self.assertLessEqual(len(chunk), limit)
            self.assertEqual(chunk, samples[offset:offset + len(chunk)])
            cursor += len(chunk)
        self.assertEqual(cursor, len(samples))

    def test_wave_duration_and_channel_limits(self):
        for channels, rate, frames, valid in [(1, 16000, 16000, True), (2, 16000, 100, False), (1, 8000, 100, False), (1, 16000, 0, False)]:
            buffer = io.BytesIO()
            with wave.open(buffer, "wb") as out:
                out.setnchannels(channels)
                out.setsampwidth(2)
                out.setframerate(rate)
                out.writeframes(b"\0\0" * channels * frames)
            buffer.seek(0)
            with wave.open(buffer, "rb") as audio:
                if valid:
                    self.assertEqual(transcribe.inspect_wave(audio), frames)
                else:
                    with self.assertRaises(transcribe.JobError):
                        transcribe.inspect_wave(audio)

    def test_oversized_media_never_invokes_decoder(self):
        with tempfile.TemporaryDirectory() as folder:
            source = Path(folder) / "media"
            with source.open("wb") as out:
                out.truncate(transcribe.MAX_MEDIA_BYTES + 1)
            with patch.object(subprocess, "run") as run:
                with self.assertRaisesRegex(transcribe.JobError, "invalid_media"):
                    transcribe.decode_audio(source, Path(folder) / "decoded.wav", "/usr/bin/ffmpeg")
                run.assert_not_called()

    def test_decoder_errors_do_not_leak_details(self):
        with tempfile.TemporaryDirectory() as folder:
            source = Path(folder) / "media"
            source.write_bytes(b"x" * 20)
            with patch.object(subprocess, "run", side_effect=subprocess.CalledProcessError(1, ["private-media-name"])):
                with self.assertRaisesRegex(transcribe.JobError, "^invalid_media$"):
                    transcribe.decode_audio(source, Path(folder) / "decoded.wav", "/usr/bin/ffmpeg")

    def test_model_verification_does_not_accept_size_alone(self):
        with tempfile.TemporaryDirectory() as folder:
            source = Path(folder) / "model"
            source.write_bytes(b"abc")
            self.assertFalse(download_models.matches(source, {"size": 3, "sha256": "0" * 64}))


if __name__ == "__main__":
    unittest.main()
