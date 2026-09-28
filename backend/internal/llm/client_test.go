package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"ninimenu/internal/config"
)

func TestStructuredReasoningOnlyAddsSupportedProviderOption(t *testing.T) {
	for _, row := range []struct {
		model, effort string
	}{
		{"glm-5.3", "low"}, {"glm-5.3-flash", "low"}, {"cook/glm-5.3", "low"},
		{"deepseek-chat", ""}, {"glm-4.7", ""}, {"custom-model", ""},
	} {
		t.Run(row.model, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				value, present := body["reasoning_effort"]
				if row.effort == "" && present || row.effort != "" && value != row.effort {
					t.Errorf("unexpected reasoning option for %s: %v", row.model, value)
				}
				if _, exists := body["thinking"]; exists {
					t.Error("must not request disabling required thinking")
				}
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ALLOW\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			settings := Settings{BaseURL: server.URL, APIKey: "fixture", Model: row.model}
			result, err := Stream(context.Background(), settings, Request{ReasoningEffort: settings.StructuredReasoningEffort()}, nil)
			if err != nil || result.Content != "ALLOW" || result.FinishReason != "stop" {
				t.Fatalf("unexpected response: %v", err)
			}
		})
	}
}

func TestResolveCPAReadsDSHOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := `{"100003":{"baseUrl":"http://172.22.0.1:8317","apiKey":"cpa-test-key","model":"glm-5.3","completionsPath":"v1/chat/completions"}}`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	previous := config.C
	config.C.CPAConfigPath = path
	config.C.CPABaseURL = ""
	config.C.CPAModel = ""
	t.Cleanup(func() { config.C = previous })

	got, ok := resolveCPA()
	if !ok {
		t.Fatal("resolveCPA() returned disabled")
	}
	if got.APIKey != "cpa-test-key" || got.BaseURL != "http://172.22.0.1:8317/v1" || got.Model != "glm-5.3" {
		t.Fatalf("unexpected CPA settings: %+v", got)
	}
	config.C.CPAModel = "cook/glm-5.3"
	got, ok = resolveCPA()
	if !ok || got.Model != "cook/glm-5.3" || got.BaseURL != "http://172.22.0.1:8317/v1" || got.APIKey != "cpa-test-key" {
		t.Fatal("server model override changed credentials or was ignored")
	}
}

func TestResolveCPAReadsRawConfigAsFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := `api-keys:
  - cpa-test-key
codex-api-key:
  - api-key: upstream-key
    models:
      - name: gpt-5.5
      - name: gpt-6-sol
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	previous := config.C
	config.C.CPAConfigPath = path
	config.C.CPABaseURL = "http://cpa.example/v1"
	config.C.CPAModel = ""
	t.Cleanup(func() { config.C = previous })

	got, ok := resolveCPA()
	if !ok || got.Model != "gpt-5.5" || got.BaseURL != "http://cpa.example/v1" {
		t.Fatalf("unexpected raw CPA settings: %+v, enabled=%v", got, ok)
	}
}

func TestCPAOverrideCanUseContainerBaseURL(t *testing.T) {
	got, ok := parseCPAOverride([]byte(`{"100003":{"baseUrl":"http://172.22.0.1:8317","apiKey":"cpa-test-key","model":"glm-5.3","completionsPath":"v1/chat/completions"}}`), "http://host.docker.internal:8317/v1")
	if !ok || got.BaseURL != "http://host.docker.internal:8317/v1" {
		t.Fatalf("unexpected container CPA base URL: %+v, enabled=%v", got, ok)
	}
}

func TestResolveCPAFallsBackWhenConfigIsUnavailable(t *testing.T) {
	previous := config.C
	config.C.CPAConfigPath = filepath.Join(t.TempDir(), "missing.yaml")
	config.C.CPABaseURL = "http://cpa.example/v1"
	t.Cleanup(func() { config.C = previous })

	if _, ok := resolveCPA(); ok {
		t.Fatal("resolveCPA() enabled with a missing config file")
	}
}
