package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"ninimenu/internal/llm"
	"strings"
	"sync/atomic"
	"testing"
)

func TestReviewDiagnosticsPreserveFailureClosureAndPrivacy(t *testing.T) {
	const private = "PRIVATE_REVIEW_SOURCE_987"
	var logs bytes.Buffer
	previousOutput := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previousOutput) })
	for _, row := range []struct {
		name, content, finish, reason, loggedFinish string
		tool, upstream, allowed                     bool
	}{
		{name: "allow", content: "ALLOW", finish: "stop", allowed: true},
		{name: "block", content: "BLOCK", finish: "stop"},
		{name: "reasoning_truncated", finish: "length", reason: "incomplete", loggedFinish: "length"},
		{name: "visible_verdict_truncated", content: "ALLOW", finish: "length", reason: "incomplete", loggedFinish: "length"},
		{name: "invalid_verdict", content: private, finish: "stop", reason: "invalid_verdict", loggedFinish: "stop"},
		{name: "missing_finish", content: "ALLOW", reason: "incomplete", loggedFinish: "missing"},
		{name: "untrusted_finish", content: "ALLOW", finish: private, reason: "incomplete", loggedFinish: "other"},
		{name: "unexpected_tools", content: "ALLOW", finish: "stop", tool: true, reason: "unexpected_tools", loggedFinish: "stop"},
		{name: "upstream_failure", upstream: true, reason: "upstream", loggedFinish: "missing"},
	} {
		t.Run(row.name, func(t *testing.T) {
			logs.Reset()
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if row.upstream {
					w.WriteHeader(http.StatusBadGateway)
					fmt.Fprint(w, private)
					return
				}
				delta := map[string]any{"content": row.content, "reasoning_content": private}
				if row.tool {
					delta["tool_calls"] = []any{map[string]any{"index": 0, "id": private, "function": map[string]string{"name": private, "arguments": private}}}
				}
				body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": delta, "finish_reason": row.finish}}})
				fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", body)
			}))
			defer server.Close()
			allowed, err := reviewContent(context.Background(), llm.Settings{BaseURL: server.URL, APIKey: private, Model: "fixture"}, "video-output", private, nil)
			if allowed != row.allowed || (err != nil) != (row.reason != "") || calls.Load() != 1 {
				t.Fatalf("unexpected review: allowed=%v error=%v calls=%d", allowed, err, calls.Load())
			}
			if row.reason == "" {
				if logs.Len() != 0 {
					t.Fatal("normal verdict was logged as a failure")
				}
			} else if !strings.Contains(logs.String(), "stage=video-output reason="+row.reason+" finish="+row.loggedFinish) {
				t.Fatalf("failure metadata missing: %s", logs.String())
			}
			if strings.Contains(logs.String(), private) || err != nil && strings.Contains(err.Error(), private) {
				t.Fatal("review diagnostics exposed private content")
			}
		})
	}
}
