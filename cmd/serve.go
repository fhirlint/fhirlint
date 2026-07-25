package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fhirlint/fhirlint/internal/profiles"
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
		"FHIR version to validate against (4.0.1, 4.3.0, 5.0.0)")
	serveCmd.Flags().StringArrayVar(&flagServeIG, "ig", nil, "IG package (or alias) to load, e.g. hl7.fhir.us.core#9.0.0 or us-core (repeatable)")
	serveCmd.Flags().BoolVar(&flagServeNoTerminologyServer, "no-terminology-server", false, "Disable the terminology server (-tx n/a)")
	serveCmd.Flags().StringVar(&flagServeTerminologyServer, "terminology-server", "", "Terminology server URL")
	serveCmd.Flags().StringVar(&flagServeTxCache, "tx-cache", "", "Terminology cache directory")

	noFile := cobra.ShellCompDirectiveNoFileComp
	_ = serveCmd.RegisterFlagCompletionFunc("fhir-version", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"4.0.1", "4.3.0", "5.0.0"}, noFile
	})
}

func runServe(_ *cobra.Command, _ []string) error {
	cfg := validator.ServerConfig{
		Port:                flagServePort,
		FHIRVersion:         flagServeFHIRVersion,
		IGs:                 resolveIGs(flagServeIG),
		NoTerminologyServer: flagServeNoTerminologyServer,
		TerminologyServer:   flagServeTerminologyServer,
		TxCache:             flagServeTxCache,
		JARPath:             viper.GetString("jar"),
		ValidatorVersion:    viper.GetString("validator-version"),
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
	out := make([]string, 0, len(in))
	for _, ig := range in {
		out = append(out, profiles.Resolve(ig))
	}
	return out
}
