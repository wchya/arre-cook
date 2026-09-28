package video

import "testing"

func TestSubtitlePreservesNumericCueText(t *testing.T) {
	const want = "今天做番茄炖牛腩\n准备牛腩\n500\n克，清水没过食材\n小火炖\n30\n分钟后出锅"
	for _, sample := range []struct{ name, raw string }{
		{"srt", "1\r\n00:00:00,000 --> 00:00:03,000\r\n今天做番茄炖牛腩\r\n准备牛腩\r\n500\r\n克，清水没过食材\r\n\r\n2\r\n00:00:03,000 --> 00:00:06,000\r\n小火炖\r\n30\r\n分钟后出锅\r\n"},
		{"vtt", "WEBVTT\n\n1\n00:00.000 --> 00:03.000\n今天做番茄炖牛腩\n准备牛腩\n500\n克，清水没过食材\n\n2\n00:03.000 --> 00:06.000\n小火炖\n30\n分钟后出锅\n"},
	} {
		t.Run(sample.name, func(t *testing.T) {
			got, err := parseSubtitle([]byte(sample.raw))
			if err != nil || got != want {
				t.Fatalf("subtitle quantities or times were changed: got %q, error %v", got, err)
			}
		})
	}
}
