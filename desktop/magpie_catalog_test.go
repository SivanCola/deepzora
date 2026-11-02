package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"deepzora/internal/config"
)

func TestMagpieAdditionDiscoversAndEnablesGatewayModels(t *testing.T) {
	for _, api := range []string{"connection", "settings"} {
		t.Run(api, func(t *testing.T) {
			isolateDesktopUserDirs(t)
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer magpie-deepzora" {
					t.Errorf("unexpected discovery %s, auth present=%t", r.URL.Path, r.Header.Get("Authorization") != "")
					http.Error(w, "unexpected request", http.StatusBadRequest)
					return
				}
				fmt.Fprint(w, `{"data":[{"id":"opencode-go/deepseek-v4-pro","display_name":"DeepSeek V4 Pro","magpie_label":"DeepSeek V4 Pro · OpenCode Go"},{"id":"codex/test-model","display_name":"Test Model","modalities":{"input":["text","image"]}},{"id":"text-embedding-3-small"}]}`)
			}))
			defer server.Close()
			a := NewApp()
			before := a.Settings()
			key := "magpie-deepzora"
			change := ModelSettingsChange{Kind: "connection_add", RequestID: "magpie-discovery", ExpectedFingerprint: before.ModelSettingsFingerprint, PresetID: "magpie", Key: &key, BaseURL: server.URL + "/v1"}
			if api == "settings" {
				result := a.ApplyModelSettings(change)
				if !result.Persisted {
					t.Fatalf("addition failed: %+v", result.Issues)
				}
				if replay := a.ApplyModelSettings(change); !replay.Persisted || calls != 1 {
					t.Fatalf("replay rediscovered or failed: %+v calls=%d", replay.Issues, calls)
				}
			} else if _, err := a.AddProviderConnectionWithURL("magpie", "", key, server.URL+"/v1"); err != nil {
				t.Fatal(err)
			}
			// A fresh app/resolver must recover labels from disk without another
			// discovery, while the stable provider/model routing IDs stay intact.
			fresh := NewApp()
			settings := fresh.Settings()
			var name string
			for _, p := range settings.Providers {
				if p.PresetID != "magpie" {
					continue
				}
				name = p.Name
				want := []string{"codex/test-model", "opencode-go/deepseek-v4-pro"}
				if !reflect.DeepEqual(p.Models, want) || p.Default != want[0] {
					t.Fatalf("gateway models not installed: %v default=%q", p.Models, p.Default)
				}
				if p.ModelCapabilities[1].DisplayName != "DeepSeek V4 Pro · OpenCode Go" {
					t.Fatalf("settings lost gateway label: %+v", p.ModelCapabilities)
				}
			}
			if name == "" {
				t.Fatal("missing Magpie connection")
			}
			models := fresh.Models()
			if len(models) != 2 || models[1].Ref != name+"/opencode-go/deepseek-v4-pro" || models[1].ModelDisplayName != "DeepSeek V4 Pro · OpenCode Go" || !models[0].Vision {
				t.Fatalf("picker metadata/routing lost: %+v", models)
			}
			if calls != 1 {
				t.Fatalf("unexpected implicit rediscovery: %d", calls)
			}
		})
	}
}

func TestMagpieDiscoveryFailureLeavesConnectionAndCredentialsUntouched(t *testing.T) {
	for _, response := range []string{"failure", "empty"} {
		t.Run(response, func(t *testing.T) {
			isolateDesktopUserDirs(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if response == "failure" {
					http.Error(w, "gateway unavailable", http.StatusServiceUnavailable)
					return
				}
				fmt.Fprint(w, `{"data":[]}`)
			}))
			defer server.Close()
			a := NewApp()
			before := a.Settings()
			credentials := config.CredentialStoreRevision()
			key := "draft-only"
			result := a.ApplyModelSettings(ModelSettingsChange{Kind: "connection_add", RequestID: "magpie-failed", ExpectedFingerprint: before.ModelSettingsFingerprint, PresetID: "magpie", BaseURL: server.URL + "/v1", Key: &key})
			if result.Persisted || len(result.Issues) == 0 || result.Issues[0].Code != "discovery_failed" {
				t.Fatalf("discovery failure hidden: %+v", result)
			}
			after := a.Settings()
			if !reflect.DeepEqual(before.Providers, after.Providers) || credentials != config.CredentialStoreRevision() {
				t.Fatal("failed discovery changed providers or credentials")
			}
		})
	}
}
