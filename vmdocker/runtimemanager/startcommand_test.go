package runtimemanager

import (
	"reflect"
	"testing"
)

func TestRuntimeStartCommandOrDefaultUsesAdapterBinary(t *testing.T) {
	got := runtimeStartCommandOrDefault("")
	want := "/usr/local/bin/vmdocker-agent"
	if got != want {
		t.Fatalf("runtimeStartCommandOrDefault(\"\") = %q, want %q", got, want)
	}
}

func TestBuildForegroundRuntimeCommandDefaultsToAdapterBinary(t *testing.T) {
	got, err := buildForegroundRuntimeCommand("")
	if err != nil {
		t.Fatalf("buildForegroundRuntimeCommand(\"\") returned error: %v", err)
	}
	want := []string{"/usr/local/bin/vmdocker-agent"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildForegroundRuntimeCommand(\"\") = %v, want %v", got, want)
	}
}
