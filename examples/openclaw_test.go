package main

import "testing"

func TestBuildOpenclawSpawnTags(t *testing.T) {
	t.Setenv("OPENCLAW_DEFAULT_MODEL", "opencode-go/kimi-k2.5")
	t.Setenv("OPENCLAW_DEFAULT_PROVIDER", "")

	tags := buildOpenclawSpawnTags(
		"kimi-coding/plan",
		"zen",
		"api-key-1",
		"gateway-token-1",
		"sandbox",
	)

	want := map[string]string{
		"provider":                             "zen",
		"model":                                "kimi-coding/plan",
		"apiKey":                               "api-key-1",
		"Container-Env-OPENCLAW_DEFAULT_MODEL": "opencode-go/kimi-k2.5",
		"Container-Env-OPENCLAW_DEFAULT_PROVIDER": "zen",
		"Container-Env-OPENCLAW_GATEWAY_TOKEN":    "gateway-token-1",
		"Runtime-Backend":                         "sandbox",
	}

	if len(tags) != len(want) {
		t.Fatalf("len(tags) = %d, want %d", len(tags), len(want))
	}
	for _, tag := range tags {
		if expected, ok := want[tag.Name]; !ok {
			t.Fatalf("unexpected tag %q=%q", tag.Name, tag.Value)
		} else if tag.Value != expected {
			t.Fatalf("tag %q = %q, want %q", tag.Name, tag.Value, expected)
		}
		delete(want, tag.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing tags: %+v", want)
	}
}

func TestBuildOpenclawSpawnTagsWithoutProvider(t *testing.T) {
	t.Setenv("OPENCLAW_DEFAULT_MODEL", "")
	t.Setenv("OPENCLAW_DEFAULT_PROVIDER", "")

	tags := buildOpenclawSpawnTags(
		"opencode-go/kimi-k2.5",
		"",
		"",
		"gateway-token-1",
		"",
	)

	foundDefaultModel := false
	for _, tag := range tags {
		if tag.Name == "provider" {
			t.Fatalf("unexpected provider tag when provider is empty")
		}
		if tag.Name == "Container-Env-OPENCLAW_DEFAULT_MODEL" {
			foundDefaultModel = true
			if tag.Value != "opencode-go/kimi-k2.5" {
				t.Fatalf("default model tag = %q, want opencode-go/kimi-k2.5", tag.Value)
			}
		}
	}
	if !foundDefaultModel {
		t.Fatalf("expected default model env tag to be added")
	}
}
