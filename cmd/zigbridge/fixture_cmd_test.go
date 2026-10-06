package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunFixtureCommand(t *testing.T) {
	// 1. No arguments
	var stdout, stderr bytes.Buffer
	code := runFixtureCommand([]string{}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("expected exit code 1 for no arguments, got %d", code)
	}
	if !strings.Contains(stderr.String(), "Usage: zigbridge fixture") {
		t.Errorf("expected usage output, got: %s", stderr.String())
	}

	// 2. Unknown subcommand
	stdout.Reset()
	stderr.Reset()
	code = runFixtureCommand([]string{"invalid_cmd"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("expected exit code 1 for unknown subcommand, got %d", code)
	}
	if !strings.Contains(stderr.String(), "Unknown subcommand: invalid_cmd") {
		t.Errorf("expected unknown subcommand error, got: %s", stderr.String())
	}

	// 3. fixture list
	stdout.Reset()
	stderr.Reset()
	code = runFixtureCommand([]string{"list"}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("expected exit code 0 for fixture list, got %d. stderr: %s", code, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "SNZB-01P") {
		t.Errorf("expected output to contain SNZB-01P, got:\n%s", output)
	}

	// 4. fixture import without flags
	stdout.Reset()
	stderr.Reset()
	code = runFixtureCommand([]string{"import"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("expected exit code 1 for import without flags, got %d", code)
	}
	if !strings.Contains(stderr.String(), "either -url or -json must be specified") {
		t.Errorf("expected error about missing url or json, got: %s", stderr.String())
	}

	// 5. fixture import with JSON file
	tmpDir := t.TempDir()
	jsonFile := filepath.Join(tmpDir, "test_device.json")
	jsonContent := `{
		"model": "SNZB-TEST",
		"vendor": "SONOFF",
		"description": "Test Switch Device",
		"zigbeeModel": ["SNZB-TEST"],
		"supports": "action, battery"
	}`
	if err := os.WriteFile(jsonFile, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("failed to create test json: %v", err)
	}

	outDir := filepath.Join(tmpDir, "out")
	stdout.Reset()
	stderr.Reset()
	code = runFixtureCommand([]string{"import", "-json", jsonFile, "-out", outDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0 for fixture import -json, got %d. stderr: %s", code, stderr.String())
	}

	expectedFile := filepath.Join(outDir, "sonoff_snzb_test.yaml")
	if _, err := os.Stat(expectedFile); err != nil {
		t.Errorf("expected imported fixture at %s: %v", expectedFile, err)
	}
}
