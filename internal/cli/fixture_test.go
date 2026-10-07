package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFixtureCommands(t *testing.T) {
	t.Run("fixture list lists embedded fixtures", func(t *testing.T) {
		cmd := newFixtureCmd()
		var stdout, stderr bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetArgs([]string{"list"})

		if err := cmd.ExecuteContext(t.Context()); err != nil {
			t.Fatalf("expected no error for fixture list, got: %v", err)
		}

		out := stdout.String()
		if !strings.Contains(out, "SNZB-01P") {
			t.Errorf("expected stdout to contain SNZB-01P, got:\n%s", out)
		}
	})

	t.Run("fixture import without flags returns error", func(t *testing.T) {
		cmd := newFixtureCmd()
		var stdout, stderr bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetArgs([]string{"import"})

		err := cmd.ExecuteContext(t.Context())
		if err == nil {
			t.Fatal("expected error when neither --url nor --json is provided")
		}
		if !strings.Contains(err.Error(), "either --url or --json must be specified") {
			t.Errorf("expected missing flag error, got: %v", err)
		}
	})

	t.Run("fixture import with JSON file", func(t *testing.T) {
		tmpDir := t.TempDir()
		jsonFile := filepath.Join(tmpDir, "test_device.json")
		jsonContent := `{
			"model": "SNZB-TEST",
			"vendor": "SONOFF",
			"description": "Test Switch Device",
			"zigbeeModel": ["SNZB-TEST"],
			"supports": "action, battery"
		}`
		if err := os.WriteFile(jsonFile, []byte(jsonContent), 0o600); err != nil {
			t.Fatalf("failed to create test json: %v", err)
		}

		outDir := filepath.Join(tmpDir, "out")
		cmd := newFixtureCmd()
		var stdout, stderr bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetArgs([]string{"import", "--json", jsonFile, "--out", outDir})

		if err := cmd.ExecuteContext(t.Context()); err != nil {
			t.Fatalf("expected no error importing from json, got: %v", err)
		}

		expectedFile := filepath.Join(outDir, "sonoff_snzb_test.yaml")
		if _, err := os.Stat(expectedFile); err != nil {
			t.Errorf("expected imported fixture file at %s: %v", expectedFile, err)
		}
	})

	t.Run("fixture unknown subcommand returns error", func(t *testing.T) {
		cmd := newFixtureCmd()
		var stdout, stderr bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetArgs([]string{"invalid_cmd"})

		err := cmd.ExecuteContext(t.Context())
		if err == nil {
			t.Fatal("expected error for unknown subcommand")
		}
	})
}
