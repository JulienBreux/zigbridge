package main

import (
	"context"
	"flag"
	"fmt"
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
	"github.com/julienbreux/zigbridge/internal/web"
)

// Build-time variables injected via -ldflags
var (
	Version   = "1.0.0"
	Commit    = "unknown"
	BuildDate = "unknown"
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
 Zigbee-to-MQTT Bridge & Direct Binding Orchestrator
`

func main() {
	if len(os.Args) > 1 && os.Args[1] == "fixture" {
		handleFixtureCommand(os.Args[2:])
		return
	}

	configPath := flag.String("config", "", "Path to YAML configuration file (default: data/config.yaml or config.yaml)")
	logLevel := flag.String("log-level", "", "Override logging level (debug, info, warn, error)")
	showVersion := flag.Bool("version", false, "Display version and build information")
	flag.Parse()

	if *showVersion {
		fmt.Printf("zigbridge version %s (commit: %s, built: %s)\n", Version, Commit, BuildDate)
		os.Exit(0)
	}

	fmt.Print(banner)
	log.Printf("[MAIN] Zigbridge v%s (commit: %s) starting up...", Version, Commit)

	// 1. Load configuration
	activeConfigPath := config.ResolveConfigPath(*configPath)
	var cfg *config.Config
	if _, err := os.Stat(activeConfigPath); err == nil {
		log.Printf("[MAIN] Loading configuration from %s", activeConfigPath)
		loaded, err := config.Load(activeConfigPath)
		if err != nil {
			log.Fatalf("[FATAL] Failed to load configuration from %s: %v", activeConfigPath, err)
		}
		cfg = loaded
	} else if _, err := os.Stat("config.yaml.dist"); err == nil {
		log.Printf("[MAIN] Config file '%s' not found, falling back to template 'config.yaml.dist'", activeConfigPath)
		loaded, err := config.Load("config.yaml.dist")
		if err != nil {
			log.Fatalf("[FATAL] Failed to load configuration from config.yaml.dist: %v", err)
		}
		cfg = loaded
	} else {
		log.Printf("[MAIN] Config file '%s' not found, applying built-in defaults", activeConfigPath)
		cfg = config.Default()
	}

	if *logLevel != "" {
		cfg.LogLevel = *logLevel
	}

	// 2. Initialize transport
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
		log.Fatalf("[FATAL] Unsupported transport type: %s", cfg.Transport.Type)
	}

	// 3. Initialize radio adapter
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
		log.Fatalf("[FATAL] Unsupported adapter type: %s", cfg.Adapter.Type)
	}

	// 4. Initialize MQTT client
	var mq mqtt.Client
	if cfg.MQTT.Enabled {
		log.Printf("[MQTT] Initializing MQTT client broker: %s (BaseTopic: %s, HA Discovery: %v)",
			cfg.MQTT.Broker, cfg.MQTT.BaseTopic, cfg.MQTT.HADiscovery)
		mq = mqtt.NewPahoClient(cfg.MQTT)
	} else {
		log.Printf("[MQTT] MQTT disabled in configuration, using virtual client")
		mq = mqtt.NewMockClient()
	}

	// 5. Initialize Central Controller
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
	}

	// 6. Forward bridge events to logger
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
			case "system_warning":
				log.Printf("[WARN] System warning: %v", evt.Payload)
			}
		}
	}()

	// 7. Initialize Web Dashboard Server
	var webSrv *web.Server
	if cfg.Web.ListenAddr != "" {
		webSrv = web.NewServer(&cfg.Web, ctrl)
	}

	// 8. Start Controller and Web Server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.Printf("[CONTROLLER] Starting coordinator controller...")
	if err := ctrl.Start(ctx); err != nil {
		log.Printf("[WARN] Controller initial start note: %v", err)
	}

	if webSrv != nil {
		log.Printf("[WEB] Starting embedded dashboard on http://%s", cfg.Web.ListenAddr)
		if err := webSrv.Start(ctx); err != nil {
			log.Fatalf("[FATAL] Failed to start web dashboard: %v", err)
		}
	}

	log.Printf("[READY] Zigbridge is operational. Press Ctrl+C to terminate.")

	// 9. Graceful shutdown handler
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	sig := <-sigCh

	log.Printf("[SHUTDOWN] Received signal '%s', initiating graceful shutdown...", sig)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if webSrv != nil {
		log.Printf("[SHUTDOWN] Stopping web server...")
		if err := webSrv.Stop(shutdownCtx); err != nil {
			log.Printf("[SHUTDOWN] Web server stop error: %v", err)
		}
	}

	log.Printf("[SHUTDOWN] Stopping Zigbridge controller and radio subsystems...")
	if err := ctrl.Stop(); err != nil {
		log.Printf("[SHUTDOWN] Controller stop error: %v", err)
	}

	log.Printf("[SHUTDOWN] Zigbridge terminated cleanly.")
}
