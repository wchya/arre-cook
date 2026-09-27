package video

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 2 && os.Args[1] == "-I" && strings.HasPrefix(filepath.Base(os.Args[2]), "__asr_helper_") {
		mode := strings.TrimPrefix(filepath.Base(os.Args[2]), "__asr_helper_")
		if os.Getenv("JWT_SECRET") != "" || os.Getenv("LLM_API_KEY") != "" || os.Getenv("PYTHONPATH") != "" {
			os.Exit(6)
		}
		input := os.Args[4]
		info, err := os.Stat(input)
		if err != nil || info.Mode().Perm() != 0600 {
			os.Exit(7)
		}
		_ = os.WriteFile(filepath.Join(filepath.Dir(input), "decoded.wav"), []byte("temporary audio"), 0600)
		switch mode {
		case "sleep":
			time.Sleep(30 * time.Second)
		case "oversize":
			_, _ = io.WriteString(os.Stdout, strings.Repeat("x", 200<<10))
			os.Exit(0)
		case "no_speech":
			_, _ = io.WriteString(os.Stdout, `{"error":"no_speech"}`)
			os.Exit(2)
		case "invalid":
			_, _ = io.WriteString(os.Stderr, "private native details")
			_, _ = io.WriteString(os.Stdout, "private malformed output")
			os.Exit(2)
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"text": transcript, "duration_seconds": 12, "segments": []any{}, "peak_rss_bytes": 1})
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func testLocalASR(t *testing.T, mode string) *LocalASR {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	l := NewLocalASR(LocalASROptions{Python: exe, Worker: filepath.Join(t.TempDir(), "__asr_helper_"+mode), FFmpeg: "/unused", ModelDir: t.TempDir(), WorkDir: t.TempDir()})
	l.readyOnce.Do(func() { l.ready = true })
	l.availableMemory = func() (int64, bool) { return 2 << 30, true }
	return l
}

var localTestMedia = append([]byte{0, 0, 0, 20}, []byte("ftypisom00000000")...)

func TestLocalASRIsolatedProcessResultAndCleanup(t *testing.T) {
	t.Setenv("JWT_SECRET", "must-not-reach-worker")
	t.Setenv("LLM_API_KEY", "must-not-reach-worker")
	t.Setenv("PYTHONPATH", "must-not-reach-worker")
	l := testLocalASR(t, "success")
	gate := &fakeGate{}
	consumed := 0
	got, err := l.transcribe(context.Background(), localTestMedia, gate, func() error { consumed++; return nil })
	if err != nil || got != transcript || consumed != 1 || gate.calls.Load() != 1 {
		t.Fatalf("result=%q err=%v consumed=%d", got, err, consumed)
	}
	entries, _ := os.ReadDir(l.options.WorkDir)
	if len(entries) != 0 {
		t.Fatal("temporary media retained")
	}
}

func TestLocalASRCancellationAndMalformedOutput(t *testing.T) {
	for _, mode := range []string{"sleep", "invalid", "oversize", "no_speech"} {
		t.Run(mode, func(t *testing.T) {
			l := testLocalASR(t, mode)
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			started := time.Now()
			_, err := l.transcribe(ctx, localTestMedia, &fakeGate{}, nil)
			if err == nil || strings.Contains(err.Error(), "private") {
				t.Fatalf("unsafe error: %v", err)
			}
			if mode == "sleep" && (!errors.Is(err, context.DeadlineExceeded) || time.Since(started) > 2*time.Second) {
				t.Fatalf("worker survived cancellation: %v", err)
			}
			if mode == "no_speech" && PublicError(err).Code != "transcript_required" {
				t.Fatalf("wrong speech fallback: %v", err)
			}
			entries, _ := os.ReadDir(l.options.WorkDir)
			if len(entries) != 0 {
				t.Fatal("failed job retained temporary media")
			}
		})
	}
}

func TestLocalASRAdmissionDoesNotConsumeQuotaOrPlatformBudget(t *testing.T) {
	l := testLocalASR(t, "success")
	gate := &fakeGate{}
	consume := func() error { t.Fatal("rejected job consumed quota"); return nil }
	localASRSlot <- struct{}{}
	_, err := l.transcribe(context.Background(), localTestMedia, gate, consume)
	<-localASRSlot
	if PublicError(err).Code != "asr_busy" {
		t.Fatalf("busy: %v", err)
	}
	l.availableMemory = func() (int64, bool) { return 100 << 20, true }
	_, err = l.transcribe(context.Background(), localTestMedia, gate, consume)
	if PublicError(err).Code != "asr_busy" {
		t.Fatalf("memory admission: %v", err)
	}
	if gate.calls.Load() != 0 {
		t.Fatal("rejected job spent network budget")
	}
}

func TestLocalModelHashAndOutputBounds(t *testing.T) {
	p := filepath.Join(t.TempDir(), "model")
	body := []byte("trusted model")
	digest := sha256.Sum256(body)
	item := modelFile{Name: "model", Size: int64(len(body)), SHA256: hex.EncodeToString(digest[:])}
	if err := os.WriteFile(p, body, 0600); err != nil {
		t.Fatal(err)
	}
	if !verifyLocalModel(p, item) {
		t.Fatal("valid model rejected")
	}
	if err := os.WriteFile(p, []byte("changed model"), 0600); err != nil {
		t.Fatal(err)
	}
	if verifyLocalModel(p, item) {
		t.Fatal("same-size model tampering accepted")
	}
	out := &limitedOutput{limit: 16}
	_, _ = io.Copy(out, strings.NewReader(strings.Repeat("x", 1000)))
	if !out.overflow || out.buffer.Len() != 16 {
		t.Fatal("output bound bypassed through io.Copy")
	}
}
