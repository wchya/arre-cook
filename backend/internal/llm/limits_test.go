package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestStreamRejectsUnboundedToolIndexesAndArguments(t *testing.T) {
	for _, body := range []string{
		`{"choices":[{"delta":{"tool_calls":[{"index":999999999,"function":{"name":"get_dish","arguments":"{}"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":-1,"function":{"name":"get_dish","arguments":"{}"}}]}}]}`,
		fmt.Sprintf(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"get_dish","arguments":%q}}]}}]}`, strings.Repeat("a", maxToolArgumentBytes+1)),
		fmt.Sprintf(`{"choices":[{"delta":{"content":%q}}]}`, strings.Repeat("x", maxContentBytes+1)),
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "data: "+body)
			fmt.Fprint(w, "\ndata: [DONE]\n\n")
		}))
		_, err := Stream(context.Background(), Settings{BaseURL: server.URL, APIKey: "private-test-key", Model: "test"}, Request{}, nil)
		server.Close()
		if err == nil {
			t.Fatal("unbounded upstream response was accepted")
		}
	}
}

func TestStreamDoesNotFollowProviderRedirectsOrExposeRawErrors(t *testing.T) {
	var reached atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached.Add(1) }))
	defer destination.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	_, err := Stream(context.Background(), Settings{BaseURL: redirect.URL, APIKey: "private-test-key", Model: "test"}, Request{}, nil)
	if err == nil || reached.Load() != 0 {
		t.Fatal("provider credential followed a redirect")
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		fmt.Fprint(w, "private-test-key SECRET_PROMPT")
	}))
	defer bad.Close()
	_, err = Stream(context.Background(), Settings{BaseURL: bad.URL, APIKey: "private-test-key", Model: "test"}, Request{}, nil)
	if err == nil || strings.Contains(err.Error(), "private-test-key") || strings.Contains(err.Error(), "SECRET_PROMPT") {
		t.Fatalf("unsafe error: %v", err)
	}
}
