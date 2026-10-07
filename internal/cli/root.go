package cli

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/julienbreux/zigbridge/internal/adapter"
	"github.com/julienbreux/zigbridge/internal/adapter/ember"
	"github.com/julienbreux/zigbridge/internal/adapter/mock"
	"github.com/julienbreux/zigbridge/internal/adapter/zstack"
	"github.com/julienbreux/zigbridge/internal/ai"
	"github.com/julienbreux/zigbridge/internal/config"
	"github.com/julienbreux/zigbridge/internal/controller"
	"github.com/julienbreux/zigbridge/internal/mqtt"
	"github.com/julienbreux/zigbridge/internal/transport"
	"github.com/julienbreux/zigbridge/internal/version"
	"github.com/julienbreux/zigbridge/internal/web"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const banner = `
  ______ _       _          _     _            
 |___  /(_)     | |        (_)   | |           
    / /  _  __ _| |__  _ __ _  __| | __ _  ___ 
   / /  | |/ _` + "`" + ` | '_ \| '__| |/ _` + "`" + ` |/ _` + "`" + ` |/ _ \
  / /__ | | (_| | |_) | |  | | (_| | (_| |  __/
 /_____|/ |\__, |_.__/|_|  |_|\__,_|\__, |\___|
      _/ | __/ |                     __/ |     
     |__/ |___/                     |___/      
 ZigBridge - Zigbee-to-MQTT Bridge & Direct Binding Orchestrator
`

// Execute runs the root command using background context.
func Execute() error {
	return ExecuteContext(context.Background())
}

// ExecuteContext runs the root command using the provided context.
func ExecuteContext(ctx context.Context) error {
	cmd := NewRootCmd(nil)
	return cmd.ExecuteContext(ctx)
}

// NewRootCmd creates a new Cobra root command configured with Viper bindings.
func NewRootCmd(v *viper.Viper) *cobra.Command {
	if v == nil {
		v = viper.New()
	}
	InitViper(v)

	var (
		cfgFile     string
		showVersion bool
	)

	cmd := &cobra.Command{
		Use:           "zigbridge",
		Short:         "ZigBridge - Zigbee-to-MQTT Bridge & Direct Binding Orchestrator",
		Long:          strings.TrimPrefix(banner, "\n"),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if showVersion {
				printF(cmd.OutOrStdout(), "zigbridge version %s (commit: %s, built: %s)\n",
					version.Version, version.Commit, version.BuildDate)
				return nil
			}

			cfg, loadedFile, err := LoadConfig(v, cfgFile)
			if err != nil {
				return err
			}

			return runBridge(cmd.Context(), cfg, loadedFile, cmd.OutOrStdout())
		},
	}

	// Persistent / Local flags
	cmd.Flags().StringVarP(&cfgFile, "config", "c", "", "Path to YAML or JSON configuration file")
	cmd.Flags().StringP("log-level", "l", "", "Override logging level (debug, info, warn, error)")
	cmd.Flags().String("web-listen-addr", "", "Web dashboard listen address (e.g. 0.0.0.0:8080)")
	cmd.Flags().String("transport-type", "", "Transport type (tcp, serial, mock)")
	cmd.Flags().String("transport-url", "", "Transport URL for TCP coordinator")
	cmd.Flags().String("transport-port", "", "Transport serial port (e.g. /dev/ttyUSB0)")
	cmd.Flags().String("mqtt-broker", "", "MQTT broker URL (e.g. tcp://localhost:1883)")
	cmd.Flags().String("adapter-type", "", "Adapter type (zstack, ember, mock)")
	cmd.Flags().Uint8("adapter-channel", 0, "Zigbee channel (11-26)")
	cmd.Flags().BoolVarP(&showVersion, "version", "v", false, "Display version and build information")

	// Bind flags to Viper keys
	_ = v.BindPFlag("log_level", cmd.Flags().Lookup("log-level"))
	_ = v.BindPFlag("web.listen_addr", cmd.Flags().Lookup("web-listen-addr"))
	_ = v.BindPFlag("transport.type", cmd.Flags().Lookup("transport-type"))
	_ = v.BindPFlag("transport.url", cmd.Flags().Lookup("transport-url"))
	_ = v.BindPFlag("transport.port", cmd.Flags().Lookup("transport-port"))
	_ = v.BindPFlag("mqtt.broker", cmd.Flags().Lookup("mqtt-broker"))
	_ = v.BindPFlag("adapter.type", cmd.Flags().Lookup("adapter-type"))
	_ = v.BindPFlag("adapter.channel", cmd.Flags().Lookup("adapter-channel"))

	// Register subcommands
	cmd.AddCommand(newFixtureCmd())
	cmd.AddCommand(newVersionCmd())

	return cmd
}

func runBridge(ctx context.Context, cfg *config.Config, loadedConfigFile string, out io.Writer) error {
	printStr(out, banner)
	log.Printf("[MAIN] ZigBridge v%s (commit: %s) starting up...", version.Version, version.Commit)

	if loadedConfigFile != "" {
		log.Printf("[MAIN] Loaded configuration from %s", loadedConfigFile)
	} else {
		log.Printf("[MAIN] No configuration file found, running with defaults and environment overrides")
	}

	// 1. Initialize transport
	var trans transport.Transport
	switch cfg.Transport.Type {
	case config.TransportTypeTCP:
		log.Printf("[TRANSPORT] Initializing TCP transport target: %s (RFC2217: %v, Keepalive: %v)",
			cfg.Transport.URL, cfg.Transport.RFC2217, cfg.Transport.TCPKeepAlive)
		trans = transport.NewTCPTransport(transport.TCPConfig{
			Address:              cfg.Transport.URL,
			KeepAlive:            cfg.Transport.TCPKeepAlive,
			ReconnectInterval:    cfg.Transport.ReconnectInterval,
			MaxReconnectInterval: cfg.Transport.MaxReconnectInterval,
			RFC2217:              cfg.Transport.RFC2217,
			ReadTimeout:          cfg.Transport.ReadTimeout,
			WriteTimeout:         cfg.Transport.WriteTimeout,
		})
	case config.TransportTypeSerial:
		log.Printf("[TRANSPORT] Initializing Serial USB transport port: %s (baudrate: %d)",
			cfg.Transport.Port, cfg.Transport.BaudRate)
		trans = transport.NewSerialTransport(transport.SerialConfig{
			Port:         cfg.Transport.Port,
			BaudRate:     cfg.Transport.BaudRate,
			ReadTimeout:  cfg.Transport.ReadTimeout,
			WriteTimeout: cfg.Transport.WriteTimeout,
		})
	case config.TransportTypeMock:
		log.Printf("[TRANSPORT] Initializing Virtual Mock Transport")
		mockTrans, _ := transport.NewMockTransport()
		trans = mockTrans
	default:
		return fmt.Errorf("unsupported transport type: %s", cfg.Transport.Type)
	}

	// 2. Initialize radio adapter
	var adp adapter.Adapter
	switch cfg.Adapter.Type {
	case config.AdapterTypeZStack:
		log.Printf("[RADIO] Initializing TI Z-Stack adapter (Channel: %d, PanID: 0x%04X, ExtPanID: %s)",
			cfg.Adapter.Channel, cfg.Adapter.PanID, cfg.Adapter.ExtPanID)
		adp = zstack.New(cfg.Adapter.Channel, cfg.Adapter.PanID, cfg.Adapter.ExtPanID)
	case config.AdapterTypeEmber:
		log.Printf("[RADIO] Initializing Silicon Labs Ember/EZSP adapter (Channel: %d, PanID: 0x%04X, ExtPanID: %s)",
			cfg.Adapter.Channel, cfg.Adapter.PanID, cfg.Adapter.ExtPanID)
		adp = ember.New(cfg.Adapter.Channel, cfg.Adapter.PanID, cfg.Adapter.ExtPanID)
	case config.AdapterTypeMock:
		log.Printf("[RADIO] Initializing Virtual Mock Adapter (Channel: %d, PanID: 0x%04X)",
			cfg.Adapter.Channel, cfg.Adapter.PanID)
		adp = mock.New(cfg.Adapter.Channel, cfg.Adapter.PanID)
	default:
		return fmt.Errorf("unsupported adapter type: %s", cfg.Adapter.Type)
	}

	// 3. Initialize MQTT client
	var mq mqtt.Client
	if cfg.MQTT.Enabled {
		log.Printf("[MQTT] Initializing MQTT client broker: %s (BaseTopic: %s, HA Discovery: %v)",
			cfg.MQTT.Broker, cfg.MQTT.BaseTopic, cfg.MQTT.HADiscovery)
		mq = mqtt.NewPahoClient(cfg.MQTT)
	} else {
		log.Printf("[MQTT] MQTT disabled in configuration, using virtual client")
		mq = mqtt.NewMockClient()
	}

	// 4. Initialize Central Controller
	ctrl := controller.New(cfg, trans, adp, mq)

	// Configure AI hook if specified
	if cfg.AI.Enabled {
		if strings.EqualFold(cfg.AI.Engine, "external_llm") && cfg.AI.LLMEndpoint != "" {
			log.Printf("[AI] Configuring external LLM analysis hook at: %s", cfg.AI.LLMEndpoint)
			ctrl.SetAnalyzer(ai.NewLLMAnalyzerHook(cfg.AI.LLMEndpoint, cfg.AI.LLMAPIKey))
		} else {
			log.Printf("[AI] Configured rule-based direct binding recommendation engine (confidence threshold: %.2f)",
				cfg.AI.MinConfidence)
		}
	} else {
		log.Printf("[AI] AI telemetry collection and recommendation engine are disabled (enable via ai.enabled in config)")
	}

	// 5. Forward bridge events to logger
	evtCh := ctrl.EventBus().Subscribe(100)
	go func() {
		for evt := range evtCh {
			switch evt.Type {
			case "device_join":
				log.Printf("[EVENT:JOIN] New device registered: %+v", evt.Payload)
			case "permit_join":
				log.Printf("[EVENT:PERMIT_JOIN] Permit join status: %+v", evt.Payload)
			case "binding_change":
				log.Printf("[EVENT:BINDING] Binding change: %+v", evt.Payload)
			case "system_info":
				log.Printf("[INFO] %v", evt.Payload)
			case "system_warning":
				log.Printf("[WARN] System warning: %v", evt.Payload)
			}
		}
	}()

	// 6. Initialize Web Dashboard Server
	var webSrv *web.Server
	if cfg.Web.ListenAddr != "" {
		webSrv = web.NewServer(&cfg.Web, ctrl)
	}

	// 7. Start Controller and Web Server
	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()

	log.Printf("[CONTROLLER] Starting coordinator controller...")
	if err := ctrl.Start(runCtx); err != nil {
		log.Printf("[WARN] Controller initial start note: %v", err)
	}

	if webSrv != nil {
		log.Printf("[WEB] Starting embedded dashboard on http://%s", cfg.Web.ListenAddr)
		if err := webSrv.Start(runCtx); err != nil {
			return fmt.Errorf("failed to start web dashboard: %w", err)
		}
	}

	log.Printf("[READY] ZigBridge is operational. Press Ctrl+C to terminate.")

	// 8. Graceful shutdown handler
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Printf("[SHUTDOWN] Received signal '%s', initiating graceful shutdown...", sig)
	case <-ctx.Done():
		log.Printf("[SHUTDOWN] Context canceled, initiating graceful shutdown...")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer shutdownCancel()

	if webSrv != nil {
		log.Printf("[SHUTDOWN] Stopping web server...")
		if err := webSrv.Stop(shutdownCtx); err != nil {
			log.Printf("[SHUTDOWN] Web server stop error: %v", err)
		}
	}

	log.Printf("[SHUTDOWN] Stopping ZigBridge controller and radio subsystems...")
	if err := ctrl.Stop(); err != nil {
		log.Printf("[SHUTDOWN] Controller stop error: %v", err)
	}

	log.Printf("[SHUTDOWN] ZigBridge terminated cleanly.")
	return nil
}
