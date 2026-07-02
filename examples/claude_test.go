package main

import (
	"os"
	"testing"
)

func TestBuildClaudeSpawnTags(t *testing.T) {
	tags := buildClaudeSpawnTags(
		"anthropic-key-1",
		"https://anthropic-proxy.example.com",
		"claude-sonnet-4-20250514",
		"--dangerously-skip-permissions",
		"sandbox",
	)

	want := map[string]string{
		"Container-Env-RUNTIME_TYPE":       "claude",
		"Container-Env-ANTHROPIC_API_KEY":  "anthropic-key-1",
		"Container-Env-ANTHROPIC_BASE_URL": "https://anthropic-proxy.example.com",
		"Container-Env-ANTHROPIC_MODEL":    "claude-sonnet-4-20250514",
		"Container-Env-CLAUDE_CODE_FLAGS":  "--dangerously-skip-permissions",
		"Runtime-Backend":                  "sandbox",
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

func TestBuildClaudeSpawnTagsMinimal(t *testing.T) {
	tags := buildClaudeSpawnTags(
		"anthropic-key-1",
		"",
		"",
		"",
		"",
	)

	want := map[string]string{
		"Container-Env-RUNTIME_TYPE":      "claude",
		"Container-Env-ANTHROPIC_API_KEY": "anthropic-key-1",
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

func TestResolveClaudeChatArgs(t *testing.T) {
	t.Setenv("CLAUDE_CHAT_COMMAND", "env message")

	tests := []struct {
		name          string
		args          []string
		wantTarget    string
		wantCommand   string
		wantShouldNew bool
		wantErr       bool
	}{
		{
			name:          "spawn and use env message when no args",
			args:          nil,
			wantCommand:   "env message",
			wantShouldNew: true,
		},
		{
			name:          "pid only uses env message",
			args:          []string{"pid-123"},
			wantCommand:   "pid-123",
			wantShouldNew: true,
		},
		{
			name:          "single message always spawns",
			args:          []string{"hello claude"},
			wantCommand:   "hello claude",
			wantShouldNew: true,
		},
		{
			name:          "pid and message",
			args:          []string{"pid-123", "hello"},
			wantTarget:    "pid-123",
			wantCommand:   "hello",
			wantShouldNew: false,
		},
		{
			name:    "too many args",
			args:    []string{"a", "b", "c"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, command, shouldSpawn, err := resolveClaudeChatArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if target != tt.wantTarget {
				t.Fatalf("target = %q, want %q", target, tt.wantTarget)
			}
			if command != tt.wantCommand {
				t.Fatalf("command = %q, want %q", command, tt.wantCommand)
			}
			if shouldSpawn != tt.wantShouldNew {
				t.Fatalf("shouldSpawn = %v, want %v", shouldSpawn, tt.wantShouldNew)
			}
		})
	}
}

func TestResolveClaudeChatArgsFallsBackToDefaultMessage(t *testing.T) {
	original, hadOriginal := os.LookupEnv("CLAUDE_CHAT_COMMAND")
	os.Unsetenv("CLAUDE_CHAT_COMMAND")
	t.Cleanup(func() {
		if hadOriginal {
			os.Setenv("CLAUDE_CHAT_COMMAND", original)
			return
		}
		os.Unsetenv("CLAUDE_CHAT_COMMAND")
	})

	_, command, shouldSpawn, err := resolveClaudeChatArgs(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !shouldSpawn {
		t.Fatal("shouldSpawn = false, want true")
	}
	if command != "你好" {
		t.Fatalf("command = %q, want %q", command, "你好")
	}
}
