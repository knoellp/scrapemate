package jshttp

// Unit tests for buildChromiumArgs.
//
// These tests are browserless and run as part of the normal `go test ./...`
// suite (no integration build tag required).  They verify the backward-compat
// contract: --single-process is present by default and absent only when
// DisableSingleProcess is explicitly requested.

import (
	"slices"
	"testing"
)

func TestBuildChromiumArgs_SingleProcessDefault(t *testing.T) {
	t.Parallel()

	args := buildChromiumArgs(false, false)
	if !slices.Contains(args, "--single-process") {
		t.Error("buildChromiumArgs(false, false) must contain --single-process (backward-compat default)")
	}
}

func TestBuildChromiumArgs_SingleProcessDisabled(t *testing.T) {
	t.Parallel()

	args := buildChromiumArgs(false, true)
	if slices.Contains(args, "--single-process") {
		t.Error("buildChromiumArgs(false, true) must NOT contain --single-process when DisableSingleProcess is true")
	}
}

func TestBuildChromiumArgs_DisableImages(t *testing.T) {
	t.Parallel()

	argsOn := buildChromiumArgs(true, false)
	if !slices.Contains(argsOn, "--blink-settings=imagesEnabled=false") {
		t.Error("buildChromiumArgs(true, false) must contain --blink-settings=imagesEnabled=false")
	}

	argsOff := buildChromiumArgs(false, false)
	if slices.Contains(argsOff, "--blink-settings=imagesEnabled=false") {
		t.Error("buildChromiumArgs(false, false) must NOT contain --blink-settings=imagesEnabled=false")
	}
}

func TestBuildChromiumArgs_DisableImagesAndDisableSingleProcess(t *testing.T) {
	t.Parallel()

	// Both flags active: images disabled AND single-process removed.
	args := buildChromiumArgs(true, true)
	if slices.Contains(args, "--single-process") {
		t.Error("--single-process must be absent when disableSingleProcess=true")
	}
	if !slices.Contains(args, "--blink-settings=imagesEnabled=false") {
		t.Error("--blink-settings=imagesEnabled=false must be present when disableImages=true")
	}
}
