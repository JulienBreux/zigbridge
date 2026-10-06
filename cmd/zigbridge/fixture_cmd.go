package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/julienbreux/zigbridge/internal/fixture"
)

func printF(w io.Writer, format string, a ...any) {
	_, _ = fmt.Fprintf(w, format, a...)
}

func printLn(w io.Writer, a ...any) {
	_, _ = fmt.Fprintln(w, a...)
}

func handleFixtureCommand(args []string) {
	os.Exit(runFixtureCommand(args, os.Stdout, os.Stderr))
}

func runFixtureCommand(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printFixtureUsage(stderr)
		return 1
	}

	switch args[0] {
	case "import":
		importCmd := flag.NewFlagSet("fixture import", flag.ContinueOnError)
		importCmd.SetOutput(stderr)
		urlFlag := importCmd.String("url", "", "Zigbee2MQTT device documentation URL (e.g. https://www.zigbee2mqtt.io/devices/SNZB-01P.html)")
		jsonFlag := importCmd.String("json", "", "Path to Zigbee2MQTT JSON definition file")
		outFlag := importCmd.String("out", "fixtures/devices", "Output directory for generated YAML fixture")

		if err := importCmd.Parse(args[1:]); err != nil {
			return 1
		}

		if *urlFlag == "" && *jsonFlag == "" {
			printF(stderr, "Error: either -url or -json must be specified\n\n")
			importCmd.Usage()
			return 1
		}

		imp := fixture.NewImporter(nil)
		var def *fixture.DeviceDefinition
		var err error

		if *urlFlag != "" {
			printF(stdout, "Fetching device definition from %s...\n", *urlFlag)
			def, err = imp.ImportFromURL(*urlFlag)
			if err != nil {
				printF(stderr, "Error importing from URL: %v\n", err)
				return 1
			}
		} else if *jsonFlag != "" {
			printF(stdout, "Reading device definition from %s...\n", *jsonFlag)
			data, readErr := os.ReadFile(*jsonFlag)
			if readErr != nil {
				printF(stderr, "Error reading JSON file %s: %v\n", *jsonFlag, readErr)
				return 1
			}
			def, err = imp.ImportFromJSON(data)
			if err != nil {
				printF(stderr, "Error importing from JSON: %v\n", err)
				return 1
			}
		}

		slug := fixture.GenerateFilenameSlug(def.Device.Vendor, def.Device.Model)
		targetPath := filepath.Join(*outFlag, slug)
		if err := fixture.SaveDefinitionToFile(def, targetPath); err != nil {
			printF(stderr, "Error saving fixture file: %v\n", err)
			return 1
		}

		printF(stdout, "✓ Successfully imported %s (%s - %s)\n", def.Device.Model, def.Device.Vendor, def.Device.Description)
		printF(stdout, "  Saved fixture to: %s\n", targetPath)
		printF(stdout, "  Zigbee Models: %s\n", strings.Join(def.Device.ZigbeeModels, ", "))
		if len(def.Device.Simulations.Actions) > 0 {
			actions := make([]string, 0, len(def.Device.Simulations.Actions))
			for a := range def.Device.Simulations.Actions {
				actions = append(actions, a)
			}
			slices.Sort(actions)
			printF(stdout, "  Simulated Actions: %s\n", strings.Join(actions, ", "))
		}
		return 0

	case "list":
		listCmd := flag.NewFlagSet("fixture list", flag.ContinueOnError)
		listCmd.SetOutput(stderr)
		dirFlag := listCmd.String("dir", "fixtures/devices", "Directory containing device fixtures")

		if err := listCmd.Parse(args[1:]); err != nil {
			return 1
		}

		reg := fixture.NewRegistry()
		if err := reg.LoadEmbedded(); err != nil {
			printF(stderr, "Warning: failed to load embedded fixtures: %v\n", err)
		}
		if _, err := os.Stat(*dirFlag); err == nil {
			if err := reg.LoadFromDir(*dirFlag); err != nil {
				printF(stderr, "Warning: failed to load directory fixtures: %v\n", err)
			}
		}

		defs := reg.List()
		slices.SortFunc(defs, func(a, b *fixture.DeviceDefinition) int {
			return strings.Compare(a.Device.Model, b.Device.Model)
		})

		printF(stdout, "Available Device Fixtures (%d):\n", len(defs))
		printF(stdout, "%-15s %-12s %-30s %s\n", "MODEL", "VENDOR", "DESCRIPTION", "ACTIONS")
		printLn(stdout, strings.Repeat("-", 80))
		for _, d := range defs {
			actions := make([]string, 0, len(d.Device.Simulations.Actions))
			for a := range d.Device.Simulations.Actions {
				actions = append(actions, a)
			}
			slices.Sort(actions)
			actionsStr := strings.Join(actions, ", ")
			if actionsStr == "" {
				actionsStr = "-"
			}
			desc := d.Device.Description
			if len(desc) > 30 {
				desc = desc[:27] + "..."
			}
			printF(stdout, "%-15s %-12s %-30s %s\n", d.Device.Model, d.Device.Vendor, desc, actionsStr)
		}
		return 0

	default:
		printF(stderr, "Unknown subcommand: %s\n\n", args[0])
		printFixtureUsage(stderr)
		return 1
	}
}

func printFixtureUsage(w io.Writer) {
	printLn(w, "Usage: zigbridge fixture <command> [options]")
	printLn(w, "\nCommands:")
	printLn(w, "  import    Import device definition from Zigbee2MQTT URL or JSON file")
	printLn(w, "  list      List available device definitions")
	printLn(w, "\nRun 'zigbridge fixture <command> -h' for more details on each command.")
}
