package services

import (
	"context"
	"ninimenu/internal/config"
	"ninimenu/internal/video"
	"sync"
)

// Preview, saving video metadata and importing content share the same reader,
// bounded caches, in-flight requests and persistent platform budgets.
var publicVideoReader = video.NewReader(video.NewClient(videoBudgetGate{}))

var localVideoASR = sync.OnceValue(func() *video.LocalASR {
	return video.NewLocalASR(video.LocalASROptions{
		Python: "/opt/video-asr/bin/python3", Worker: "/app/video-asr/transcribe.py", FFmpeg: "/usr/bin/ffmpeg",
		ModelDir: config.C.VideoASRModelDir, WorkDir: config.C.VideoASRWorkDir,
	})
})

func VideoASRSettings() video.ASRSettings {
	if config.C.VideoASRProvider == "local" {
		return video.ASRSettings{Local: localVideoASR()}
	}
	if config.C.VideoASRProvider != "" && config.C.VideoASRProvider != "remote" {
		return video.ASRSettings{}
	}
	return video.ASRSettings{URL: config.C.VideoASRURL, APIKey: config.C.VideoASRAPIKey, Model: config.C.VideoASRModel}
}

func ReadVideoTranscript(ctx context.Context, raw, manual string, status func(string), beforeAI func() error) (video.Source, error) {
	return publicVideoReader.Transcript(ctx, raw, manual, VideoASRSettings(), status, beforeAI)
}
