package video

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Optional native-runtime check: run the compiled test binary in the Linux
// application image with a public, locally mounted media fixture. No API calls.
func TestLocalASRNativeIntegration(t *testing.T) {
	mediaPath := os.Getenv("ARRE_VIDEO_ASR_TEST_MEDIA")
	if mediaPath == "" {
		t.Skip("set ARRE_VIDEO_ASR_TEST_MEDIA inside the Linux application image")
	}
	data, err := os.ReadFile(mediaPath)
	if err != nil {
		t.Fatal(err)
	}
	l := NewLocalASR(LocalASROptions{Python: "/opt/video-asr/bin/python3", Worker: "/app/video-asr/transcribe.py",
		FFmpeg: "/usr/bin/ffmpeg", ModelDir: "/app/models/video-asr", WorkDir: t.TempDir()})
	if !l.Ready() {
		t.Fatal("native runtime or sandbox unavailable")
	}
	// The test container has an independent 600 MiB hard memory cap. Production
	// admission is tested separately, without loading a real model.
	l.availableMemory = func() (int64, bool) { return 2 << 30, true }
	assertClean := func(t *testing.T) {
		t.Helper()
		entries, err := os.ReadDir(l.options.WorkDir)
		if err != nil || len(entries) != 0 {
			t.Fatalf("temporary files survived: count=%d err=%v", len(entries), err)
		}
	}
	t.Run("speech", func(t *testing.T) {
		spent := 0
		text, err := l.transcribe(context.Background(), data, &fakeGate{}, func() error { spent++; return nil })
		if err != nil || spent != 1 || len([]rune(text)) < 100 || !strings.Contains(text, "鸡") {
			t.Fatalf("native transcription failed: err=%v spent=%d characters=%d", err, spent, len([]rune(text)))
		}
		assertClean(t)
	})
	t.Run("cancel", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		start := time.Now()
		_, err := l.transcribe(ctx, data, &fakeGate{}, nil)
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 4*time.Second {
			t.Fatalf("native cancellation failed: %v", err)
		}
		assertClean(t)
	})
	for _, tc := range []struct {
		name, code string
		data       []byte
	}{
		{"invalid", "unsupported_audio", localTestMedia},
		{"silence", "transcript_required", silentWAV(1)},
		{"actual-duration-limit", "too_long", silentWAV(601)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := l.transcribe(context.Background(), tc.data, &fakeGate{}, nil)
			if err == nil || PublicError(err).Code != tc.code {
				t.Fatalf("error=%v want=%s", err, tc.code)
			}
			assertClean(t)
		})
	}
	if _, err := os.Stat(filepath.Join(l.options.WorkDir, "decoded.wav")); !os.IsNotExist(err) {
		t.Fatal("decoded audio retained")
	}
}

func silentWAV(seconds int) []byte {
	data := make([]byte, 44+seconds*16000*2)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:20], 16)
	binary.LittleEndian.PutUint16(data[20:22], 1)
	binary.LittleEndian.PutUint16(data[22:24], 1)
	binary.LittleEndian.PutUint32(data[24:28], 16000)
	binary.LittleEndian.PutUint32(data[28:32], 32000)
	binary.LittleEndian.PutUint16(data[32:34], 2)
	binary.LittleEndian.PutUint16(data[34:36], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:44], uint32(len(data)-44))
	return data
}
