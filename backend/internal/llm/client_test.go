package llm

import (
	"os"
	"path/filepath"
	"testing"

	"ninimenu/internal/config"
)

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
