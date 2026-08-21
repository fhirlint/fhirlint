package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fhirlint/fhirlint/internal/profiles"
	"github.com/fhirlint/fhirlint/internal/txreplay"
	"github.com/fhirlint/fhirlint/internal/validator"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// serverStartupTimeout bounds how long to wait for the validator server to load
// its packages and become ready. IG loading can take a while, so this is generous.
const serverStartupTimeout = 10 * time.Minute

var (
	flagServePort                int
	flagServeFHIRVersion         string
	flagServeIG                  []string
	flagServeNoTerminologyServer bool
	flagServeTerminologyServer   string
	flagServeTxCache             string
	flagServeTxOffline           bool
	flagServeTxDir               string
	flagServeOffline             bool
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run a persistent validator HTTP server (warm validator for fast repeated runs)",
	Long: `Start a long-lived validator that loads packages and terminology once and
keeps them warm, so repeated validations avoid JVM and package-load startup cost.

The server's FHIR version, IGs and terminology settings are fixed for its
lifetime. Load the IGs you need with --ig; clients then select profiles from
those IGs per request. Point clients at it with:

  fhirlint validate ./fhir/ --server http://localhost:8080

The server runs until interrupted with Ctrl-C.

Examples:
  fhirlint serve
  fhirlint serve --port 9000 --ig hl7.fhir.us.core#9.0.0
  fhirlint serve --fhir-version 4.0.1 --ig us-core --no-terminology-server`,
	Args:         cobra.NoArgs,
	RunE:         runServe,
	SilenceUsage: true,
}

func init() {
	serveCmd.Flags().IntVar(&flagServePort, "port", 8080, "Port to listen on")
	serveCmd.Flags().StringVar(&flagServeFHIRVersion, "fhir-version", defaultFHIRVersion,
		"FHIR version to validate against ("+validator.FHIRVersionList()+")")
	serveCmd.Flags().StringArrayVar(&flagServeIG, "ig", nil, "IG package (or alias) to load, e.g. hl7.fhir.us.core#9.0.0 or us-core (repeatable)")
	serveCmd.Flags().BoolVar(&flagServeNoTerminologyServer, "no-terminology-server", false, "Disable the terminology server (-tx n/a)")
	serveCmd.Flags().StringVar(&flagServeTerminologyServer, "terminology-server", "", "Terminology server URL")
	serveCmd.Flags().StringVar(&flagServeTxCache, "tx-cache", "", "Terminology cache directory")
	serveCmd.Flags().BoolVar(&flagServeTxOffline, "tx-offline", false,
		"Replay terminology responses recorded by 'fhirlint tx warm' instead of contacting a server")
	serveCmd.Flags().StringVar(&flagServeTxDir, "tx-dir", "",
		"Directory holding the terminology recording (default: "+txreplay.DefaultDir+"/)")
	serveCmd.Flags().BoolVar(&flagServeOffline, "offline", false,
		"Forbid all network access: cached JAR, cached IG packages, and the validator's own HTTP blocked")

	noFile := cobra.ShellCompDirectiveNoFileComp
	_ = serveCmd.RegisterFlagCompletionFunc("fhir-version", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return validator.FHIRVersionIDs(), noFile
	})
}

func runServe(cmd *cobra.Command, _ []string) error {
	if flagServeOffline && cmd.Flags().Changed("terminology-server") {
		return fmt.Errorf("--offline forbids network access, so it cannot be combined with --terminology-server — " +
			"drop one, or replay a recording with --tx-offline")
	}

	// Offline terminology: stand in for the terminology server for the whole
	// lifetime of the validator server.
	var txPlayer *txreplay.Server
	txServer := flagServeTerminologyServer
	txCache := flagServeTxCache
	txSettings := ""
	if flagServeTxOffline {
		switch {
		case flagServeNoTerminologyServer:
			return fmt.Errorf("--tx-offline and --no-terminology-server are mutually exclusive: one replays terminology, the other skips it")
		case cmd.Flags().Changed("terminology-server"):
			return fmt.Errorf("--tx-offline replaces the terminology server; drop --terminology-server or record against it with 'fhirlint tx warm'")
		}
		dir := txRecordingDir(flagServeTxDir)
		store, err := txreplay.Open(dir)
		if err != nil {
			return err
		}
		if store.Len() == 0 {
			return fmt.Errorf("no terminology recording in %s/ — record one first with: fhirlint tx warm <path>", dir)
		}
		txPlayer = txreplay.NewPlayer(store)
		baseURL, err := txPlayer.Start()
		if err != nil {
			return err
		}
		defer func() { _ = txPlayer.Stop() }()
		// Validator 6.10.0+ refuses plain-HTTP destinations; exempt only this
		// loopback replay server.
		settingsPath, cleanupSettings, serr := txreplay.WriteJARSettings(baseURL)
		if serr != nil {
			return serr
		}
		defer cleanupSettings()
		txSettings = settingsPath
		txServer = baseURL
		// Every request has to reach the replay server, or an incomplete
		// recording stays invisible behind the JAR's own cache.
		txCache = "n/a"
		fmt.Fprintf(os.Stderr, "Replaying %d recorded terminology interaction(s) from %s/\n", store.Len(), dir)
	}

	cfg := validator.ServerConfig{
		Port:                flagServePort,
		FHIRVersion:         flagServeFHIRVersion,
		IGs:                 resolveIGs(flagServeIG),
		NoTerminologyServer: flagServeNoTerminologyServer,
		TerminologyServer:   txServer,
		TxCache:             txCache,
		FHIRSettings:        txSettings,
		JARPath:             viper.GetString("jar"),
		Proxy:               validator.ProxyConfig{Proxy: viper.GetString("proxy"), HTTPSProxy: viper.GetString("https-proxy")},
		ValidatorVersion:    viper.GetString("validator-version"),
	}

	if flagServeOffline {
		if err := applyOfflineServer(&cfg, os.Stderr); err != nil {
			return err
		}
	}

	fmt.Fprintf(os.Stderr, "Starting validator server on port %d (FHIR %s)…\n", cfg.Port, cfg.FHIRVersion)
	srv, err := validator.StartServer(cfg, os.Stderr, serverStartupTimeout)
	if err != nil {
		return &exitErr{code: 2, err: err}
	}

	fmt.Printf("fhirlint validator server ready at %s\n", srv.URL())
	fmt.Printf("Validate against it with:  fhirlint validate <path> --server %s\n", srv.URL())
	fmt.Fprintln(os.Stderr, "Press Ctrl-C to stop.")

	// Wait for a signal or for the server to exit on its own.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	exited := make(chan error, 1)
	go func() { exited <- srv.Wait() }()

	// Unlike a one-shot validate, a long-lived server cannot fail the run on a
	// replay miss — the request has already been answered. Report them at
	// shutdown so an incomplete recording still surfaces.
	defer func() {
		if txPlayer == nil {
			return
		}
		if misses := txPlayer.Misses(); len(misses) > 0 {
			fmt.Fprintf(os.Stderr, "\nwarn: %d terminology request(s) were not in the recording:\n", len(misses))
			for i, m := range misses {
				if i == maxReportedMisses {
					fmt.Fprintf(os.Stderr, "  … and %d more\n", len(misses)-maxReportedMisses)
					break
				}
				fmt.Fprintf(os.Stderr, "  %s\n", m)
			}
			fmt.Fprintln(os.Stderr, "Re-record with: fhirlint tx warm <path>")
		}
	}()

	select {
	case <-sigCh:
		fmt.Fprintln(os.Stderr, "\nStopping validator server…")
		_ = srv.Stop()
		return nil
	case werr := <-exited:
		if werr != nil {
			return &exitErr{code: 2, err: fmt.Errorf("validator server exited: %w", werr)}
		}
		return nil
	}
}

// resolveIGs resolves any alias-like IG values (aliases containing '#' pass through).
func resolveIGs(in []string) []string {
	return profiles.ResolveAll(in)
}
