package services

import (
	"context"
	"ninimenu/internal/config"
	"ninimenu/internal/video"
)

// Preview, saving video metadata and importing content share the same reader,
// bounded caches, in-flight requests and persistent platform budgets.
var publicVideoReader = video.NewReader(video.NewClient(videoBudgetGate{}))

func VideoASRSettings() video.ASRSettings {
	return video.ASRSettings{URL: config.C.VideoASRURL, APIKey: config.C.VideoASRAPIKey, Model: config.C.VideoASRModel}
}

func ReadVideoTranscript(ctx context.Context, raw, manual string, status func(string), beforeAI func() error) (video.Source, error) {
	return publicVideoReader.Transcript(ctx, raw, manual, VideoASRSettings(), status, beforeAI)
}
