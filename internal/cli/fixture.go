package cli

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/julienbreux/zigbridge/internal/fixture"
	"github.com/spf13/cobra"
)

func newFixtureCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fixture",
		Short: "Manage device fixtures and simulations",
	}

	cmd.AddCommand(newFixtureImportCmd())
	cmd.AddCommand(newFixtureListCmd())

	return cmd
}

func newFixtureImportCmd() *cobra.Command {
	var (
		urlFlag  string
		jsonFlag string
		outFlag  string
	)

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import device definition from Zigbee2MQTT URL or JSON file",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if urlFlag == "" && jsonFlag == "" {
				return errors.New("either --url or --json must be specified")
			}

			stdout := cmd.OutOrStdout()
			stderr := cmd.ErrOrStderr()

			imp := fixture.NewImporter(nil)
			var def *fixture.DeviceDefinition
			var err error

			if urlFlag != "" {
				printF(stdout, "Fetching device definition from %s...\n", urlFlag)
				def, err = imp.ImportFromURL(urlFlag)
				if err != nil {
					printF(stderr, "Error importing from URL: %v\n", err)
					return err
				}
			} else if jsonFlag != "" {
				printF(stdout, "Reading device definition from %s...\n", jsonFlag)
				data, readErr := os.ReadFile(jsonFlag)
				if readErr != nil {
					printF(stderr, "Error reading JSON file %s: %v\n", jsonFlag, readErr)
					return readErr
				}
				def, err = imp.ImportFromJSON(data)
				if err != nil {
					printF(stderr, "Error importing from JSON: %v\n", err)
					return err
				}
			}

			slug := fixture.GenerateFilenameSlug(def.Device.Vendor, def.Device.Model)
			targetPath := filepath.Join(outFlag, slug)
			if err := fixture.SaveDefinitionToFile(def, targetPath); err != nil {
				printF(stderr, "Error saving fixture file: %v\n", err)
				return err
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
			return nil
		},
	}

	cmd.Flags().StringVar(&urlFlag, "url", "", "Zigbee2MQTT device documentation URL (e.g. https://www.zigbee2mqtt.io/devices/SNZB-01P.html)")
	cmd.Flags().StringVar(&jsonFlag, "json", "", "Path to Zigbee2MQTT JSON definition file")
	cmd.Flags().StringVar(&outFlag, "out", "fixtures/devices", "Output directory for generated YAML fixture")

	return cmd
}

func newFixtureListCmd() *cobra.Command {
	var dirFlag string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available device definitions",
		RunE: func(cmd *cobra.Command, _ []string) error {
			stdout := cmd.OutOrStdout()
			stderr := cmd.ErrOrStderr()

			reg := fixture.NewRegistry()
			if err := reg.LoadEmbedded(); err != nil {
				printF(stderr, "Warning: failed to load embedded fixtures: %v\n", err)
			}
			if _, err := os.Stat(dirFlag); err == nil {
				if err := reg.LoadFromDir(dirFlag); err != nil {
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
			return nil
		},
	}

	cmd.Flags().StringVar(&dirFlag, "dir", "fixtures/devices", "Directory containing device fixtures")

	return cmd
}
