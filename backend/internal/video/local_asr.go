package video

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"ninimenu/internal/resourcebudget"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

//go:embed model.lock.json
var localModelLock []byte

type modelFile struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

var localModel = func() struct {
	ID    string      `json:"id"`
	Files []modelFile `json:"files"`
} {
	var manifest struct {
		ID    string      `json:"id"`
		Files []modelFile `json:"files"`
	}
	if json.Unmarshal(localModelLock, &manifest) != nil || manifest.ID == "" || len(manifest.Files) != 3 {
		panic("invalid bundled ASR model manifest")
	}
	return manifest
}()

const localASRTimeout = 50 * time.Second
const localASRMaxRSS = 512 << 20
const localASRStartMemory = localASRMaxRSS + (256 << 20)

// Shared across readers/configuration objects; this application deploys one API
// instance. A second local job never waits while retaining a downloaded video.
var localASRSlot = make(chan struct{}, 1)

type LocalASROptions struct {
	Python, Worker, FFmpeg, ModelDir, WorkDir string
}

type LocalASR struct {
	options         LocalASROptions
	readyOnce       sync.Once
	ready           bool
	availableMemory func() (int64, bool)
}

func NewLocalASR(options LocalASROptions) *LocalASR {
	return &LocalASR{options: options, availableMemory: localAvailableMemory}
}

// Models are mounted read-only in production. Verify once per application start,
// including status requests, without loading a resident inference session.
func (l *LocalASR) Ready() bool {
	if l == nil {
		return false
	}
	l.readyOnce.Do(func() {
		if runtime.GOOS != "linux" {
			return
		}
		for _, p := range []string{l.options.Python, l.options.Worker, l.options.FFmpeg} {
			info, err := os.Stat(p)
			if !filepath.IsAbs(p) || err != nil || !info.Mode().IsRegular() {
				log.Print("[video-asr] disabled reason=runtime_missing")
				return
			}
		}
		if !filepath.IsAbs(l.options.ModelDir) || !filepath.IsAbs(l.options.WorkDir) {
			return
		}
		for _, item := range localModel.Files {
			if !verifyLocalModel(filepath.Join(l.options.ModelDir, item.Name), item) {
				log.Print("[video-asr] disabled reason=model_not_verified")
				return
			}
		}
		if err := os.MkdirAll(l.options.WorkDir, 0700); err != nil {
			return
		}
		probe, err := os.MkdirTemp(l.options.WorkDir, "check-")
		if err != nil {
			return
		}
		defer os.RemoveAll(probe)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, l.options.Python, "-I", l.options.Worker,
			"--input", filepath.Join(probe, "probe.wav"), "--model-dir", l.options.ModelDir,
			"--ffmpeg", l.options.FFmpeg, "--check-runtime")
		cmd.Dir, cmd.Env = probe, localASREnvironment(probe)
		configureLocalProcess(cmd)
		cmd.WaitDelay = time.Second
		out := &limitedOutput{limit: 1024}
		cmd.Stdout, cmd.Stderr = out, &limitedOutput{limit: 4096}
		var result struct {
			Ready bool `json:"ready"`
		}
		if err := cmd.Run(); err != nil || out.overflow || json.Unmarshal(out.buffer.Bytes(), &result) != nil || !result.Ready {
			log.Print("[video-asr] disabled reason=runtime_or_sandbox_unavailable")
			return
		}
		l.ready = true
	})
	return l.ready
}

func localASREnvironment(jobDir string) []string {
	return []string{"PATH=/usr/local/bin:/usr/bin:/bin", "LANG=C.UTF-8", "TMPDIR=" + jobDir,
		"OPENBLAS_NUM_THREADS=1", "OMP_NUM_THREADS=2", "MKL_NUM_THREADS=1", "MALLOC_ARENA_MAX=2"}
}

func verifyLocalModel(path string, item modelFile) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != item.Size {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(f, item.Size+1))
	return err == nil && n == item.Size && hex.EncodeToString(hash.Sum(nil)) == item.SHA256
}

type limitedOutput struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func (b *limitedOutput) Write(p []byte) (int, error) {
	n := len(p)
	room := max(0, b.limit-b.buffer.Len())
	if n > room {
		b.overflow = true
		p = p[:room]
	}
	_, _ = b.buffer.Write(p)
	return n, nil
}

func (l *LocalASR) transcribe(ctx context.Context, data []byte, gate Gate, beforeAI func() error) (string, error) {
	if !l.Ready() {
		return "", problem("asr_unavailable", "本地语音转写暂不可用，可粘贴字幕或文稿提炼")
	}
	filename, _, err := mediaFormat(data)
	if err != nil {
		return "", err
	}
	select {
	case localASRSlot <- struct{}{}:
		defer func() { <-localASRSlot }()
	default:
		return "", &Error{Code: "asr_busy", Message: "正在处理另一个视频的语音，请稍后再试", RetryAfter: 5}
	}
	release, err := resourcebudget.Acquire(ctx)
	if err != nil {
		return "", &Error{Code: "asr_busy", Message: "正在处理图片或视频，请稍后重试", RetryAfter: 5}
	}
	defer release()
	if available, ok := l.availableMemory(); !ok || available < localASRStartMemory {
		return "", problem("asr_busy", "当前可用资源不足，语音转写已暂缓；可稍后重试或粘贴文稿")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	jobDir, err := os.MkdirTemp(l.options.WorkDir, "job-")
	if err != nil {
		return "", problem("asr_unavailable", "语音转写暂不可用，请稍后重试")
	}
	defer os.RemoveAll(jobDir)
	input := filepath.Join(jobDir, filename)
	if err := os.WriteFile(input, data, 0600); err != nil {
		return "", problem("asr_unavailable", "语音转写暂不可用，请稍后重试")
	}
	if err := gate.Acquire(ctx, "asr"); err != nil {
		return "", err
	}
	if beforeAI != nil {
		if err := beforeAI(); err != nil {
			return "", err
		}
	}
	jobCtx, cancel := context.WithTimeout(ctx, localASRTimeout)
	defer cancel()
	cmd := exec.CommandContext(jobCtx, l.options.Python, "-I", l.options.Worker,
		"--input", input, "--model-dir", l.options.ModelDir, "--ffmpeg", l.options.FFmpeg)
	cmd.Dir = jobDir
	// Never expose the application's database, JWT, SMTP or LLM credentials to
	// media decoders / inference libraries. Python isolation ignores user modules.
	cmd.Env = localASREnvironment(jobDir)
	configureLocalProcess(cmd)
	cmd.WaitDelay = time.Second
	out, diagnostics := &limitedOutput{limit: 128 << 10}, &limitedOutput{limit: 4 << 10}
	cmd.Stdout, cmd.Stderr = out, diagnostics
	started := time.Now()
	if err := cmd.Start(); err != nil {
		return "", problem("asr_unavailable", "本地语音转写启动失败，可粘贴字幕提炼")
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	resourceExceeded := false
	waiting := true
	for waiting {
		select {
		case err = <-done:
			waiting = false
		case <-ticker.C:
			rss := localProcessRSS(cmd.Process.Pid)
			available, ok := l.availableMemory()
			if rss > localASRMaxRSS || (ok && available < 128<<20 && rss > 128<<20) {
				resourceExceeded = true
				cancel()
			}
		}
	}
	// Reap any descendant that survived an abnormal worker exit as well.
	_ = cmd.Cancel()
	if resourceExceeded {
		log.Print("[video-asr] stopped reason=memory_limit")
		return "", problem("asr_busy", "本次语音识别超过资源限制，已停止；可选择较短视频或粘贴文稿")
	}
	if jobCtx.Err() != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", problem("asr_timeout", "本次语音识别用时较长，已停止；可选择较短视频或粘贴文稿")
	}
	var result struct {
		Text     string            `json:"text"`
		Error    string            `json:"error"`
		Duration float64           `json:"duration_seconds"`
		PeakRSS  int64             `json:"peak_rss_bytes"`
		Segments []json.RawMessage `json:"segments"`
	}
	if out.overflow || json.Unmarshal(out.buffer.Bytes(), &result) != nil {
		return "", problem("asr_unavailable", "语音识别未能完成，可粘贴字幕或文稿提炼")
	}
	if err != nil || result.Error != "" {
		// Only stable error codes cross the process boundary, never stderr.
		switch result.Error {
		case "no_speech":
			return "", problem("transcript_required", "未识别到有效语音；仅显示在画面上的文字需要手动粘贴为文稿")
		case "duration_limit", "text_limit", "segment_limit":
			return "", problem("too_long", "视频内容超出自动识别限制，请选择较短视频或粘贴文稿")
		case "invalid_media":
			return "", problem("unsupported_audio", "无法读取视频音轨，请粘贴字幕或文稿提炼")
		default:
			log.Print("[video-asr] failed reason=worker_error")
			return "", problem("asr_unavailable", "本地语音识别未能完成，可稍后重试或粘贴文稿")
		}
	}
	if result.Duration <= 0 || result.Duration > MaxDuration || len(result.Segments) > 1000 {
		return "", problem("asr_unavailable", "语音识别结果无效，请粘贴字幕提炼")
	}
	text, err := ValidateTranscript(result.Text)
	if err == nil {
		log.Printf("[video-asr] complete model=%s elapsed_ms=%d peak_rss_mib=%d segments=%d", localModel.ID,
			time.Since(started).Milliseconds(), result.PeakRSS>>20, len(result.Segments))
	}
	return text, err
}

func (l *LocalASR) cacheKey() string { return fmt.Sprintf("local:%s", localModel.ID) }
