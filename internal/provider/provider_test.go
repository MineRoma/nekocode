package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/m1neroma/neko/internal/config"
)

func TestOpenAIListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatal("missing authorization")
		}
		fmt.Fprint(w, `{"data":[{"id":"model-b"},{"id":"model-a","context_window":32000}]}`)
	}))
	defer server.Close()
	t.Setenv("TEST_NEKO_KEY", "test-key")
	client, err := New(config.Provider{Name: "test", Compatibility: "openai", BaseURL: server.URL + "/v1", APIKeyEnv: "TEST_NEKO_KEY"})
	if err != nil {
		t.Fatal(err)
	}
	models, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].ID != "model-a" || models[0].ContextWindow != 32000 {
		t.Fatalf("unexpected models %#v", models)
	}
}

func TestAnthropicListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Fatal("missing API key")
		}
		fmt.Fprint(w, `{"data":[{"id":"claude-z"},{"id":"claude-a"}]}`)
	}))
	defer server.Close()
	t.Setenv("TEST_NEKO_KEY", "test-key")
	client, err := New(config.Provider{Name: "test", Compatibility: "anthropic", BaseURL: server.URL + "/v1", APIKeyEnv: "TEST_NEKO_KEY"})
	if err != nil {
		t.Fatal(err)
	}
	models, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].ID != "claude-a" {
		t.Fatalf("unexpected models %#v", models)
	}
}

func TestDirectAndLegacyPastedAPIKeys(t *testing.T) {
	direct := &client{cfg: config.Provider{APIKey: "direct-key"}}
	key, err := direct.apiKey()
	if err != nil || key != "direct-key" {
		t.Fatalf("direct key failed: %q, %v", key, err)
	}

	legacy := &client{cfg: config.Provider{APIKeyEnv: "sk-legacy-key"}}
	key, err = legacy.apiKey()
	if err != nil || key != "sk-legacy-key" {
		t.Fatalf("legacy pasted key failed: %q, %v", key, err)
	}
}
