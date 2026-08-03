package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/fhirlint/fhirlint/internal/profiles"
	"github.com/fhirlint/fhirlint/internal/txreplay"
	"github.com/fhirlint/fhirlint/internal/validator"
)

var (
	flagTxWarmDir         string
	flagTxWarmFHIRVersion string
	flagTxWarmIG          []string
	flagTxWarmProfile     []string
	flagTxWarmServer      string
)

var txCmd = &cobra.Command{
	Use:   "tx",
	Short: "Manage the terminology recording used by --tx-offline",
}

var txWarmCmd = &cobra.Command{
	Use:   "warm [path]",
	Short: "Record the terminology traffic a validation needs, for later offline replay",
	Long: `Record every terminology request a validation makes, so later runs can replay
them with --tx-offline instead of contacting a terminology server.

The validator is pointed at a local recording proxy that forwards to the real
terminology server and stores each request and response. Commit the resulting
directory, or rebuild it as a CI artifact, and validation becomes reproducible
and independent of the terminology server's availability.

Record the same inputs, profiles and IGs you validate with — a request that was
never recorded is an error at replay time, not a silent fallback to the network.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runTxWarm,
}

func init() {
	txCmd.AddCommand(txWarmCmd)
	rootCmd.AddCommand(txCmd)

	txWarmCmd.Flags().StringVar(&flagTxWarmDir, "tx-dir", "",
		"Directory to record into (default: "+txreplay.DefaultDir+"/)")
	txWarmCmd.Flags().StringVar(&flagTxWarmFHIRVersion, "fhir-version", "4.0.1",
		"FHIR version: 4.0.1, 4.3.0, 5.0.0")
	txWarmCmd.Flags().StringSliceVar(&flagTxWarmIG, "ig", nil,
		"IG package, e.g. kbv.basis#1.5.0 (repeatable)")
	txWarmCmd.Flags().StringSliceVarP(&flagTxWarmProfile, "profile", "p", nil,
		"Profile alias or URL (repeatable)")
	txWarmCmd.Flags().StringVar(&flagTxWarmServer, "terminology-server", "",
		"Terminology server to record from (default: the validator's own default)")
}

func runTxWarm(_ *cobra.Command, args []string) error {
	if err := validator.CheckJava(); err != nil {
		return err
	}

	arg := "."
	if len(args) > 0 {
		arg = args[0]
	}
	paths, err := collectPathsFromArgs([]string{arg}, txRecordingExcludes(flagTxWarmDir))
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("no FHIR resources found under %s", arg)
	}

	dir := txRecordingDir(flagTxWarmDir)
	store, err := txreplay.Open(dir)
	if err != nil {
		return err
	}
	before := store.Len()

	upstream := flagTxWarmServer
	if upstream == "" {
		upstream = validator.DefaultTerminologyEndpoint(flagTxWarmFHIRVersion)
	}
	rec := txreplay.NewRecorder(store, upstream, nil)
	baseURL, err := rec.Start()
	if err != nil {
		return err
	}
	defer func() { _ = rec.Stop() }()

	resolvedProfiles := make([]string, 0, len(flagTxWarmProfile))
	for _, prof := range flagTxWarmProfile {
		resolvedProfiles = append(resolvedProfiles, profiles.Resolve(prof))
	}

	fmt.Fprintf(os.Stderr, "Recording terminology traffic from %s into %s/ (%d file(s))…\n",
		upstream, dir, len(paths))

	opts := validator.Options{
		FHIRVersion:       flagTxWarmFHIRVersion,
		Profiles:          resolvedProfiles,
		IGs:               resolveIGs(flagTxWarmIG),
		TerminologyServer: baseURL,
		// Disable the JAR's own terminology cache while recording. Otherwise it
		// answers from ~/.fhir and those requests never reach the recorder, so
		// the recording would be complete only on the machine that made it.
		TxCache: "n/a",
		// The proxy is loopback HTTP by design; the warning is about sending
		// data unencrypted to a remote server, which is not what happens here.
		AllowInsecureTx:  true,
		JARPath:          viper.GetString("jar"),
		ValidatorVersion: viper.GetString("validator-version"),
		Proxy:            validator.ProxyConfig{Proxy: viper.GetString("proxy"), HTTPSProxy: viper.GetString("https-proxy")},
	}

	if _, err := validator.RunMultiple(paths, opts); err != nil {
		return fmt.Errorf("recording run failed: %w", err)
	}

	if err := store.WriteManifest(txreplay.Manifest{
		Upstream:    upstream,
		FHIRVersion: flagTxWarmFHIRVersion,
		Recorded:    time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		return err
	}

	added := store.Len() - before
	fmt.Fprintf(os.Stderr, "Recorded %d terminology interaction(s) (%d new) in %s/\n", store.Len(), added, dir)
	fmt.Fprintf(os.Stderr, "Replay them with:  fhirlint validate %s --tx-offline\n", arg)
	return nil
}

// txRecordingExcludes keeps a terminology recording out of the input set.
//
// Recordings are JSON and live inside the project — typically committed next to
// the resources they were recorded from — so without this they get picked up and
// validated as if they were FHIR resources.
func txRecordingExcludes(flagValue string) []string {
	dir := strings.TrimSuffix(filepath.ToSlash(txRecordingDir(flagValue)), "/")
	if dir == "" {
		return nil
	}
	return []string{dir + "/**"}
}

// txRecordingDir resolves the recording directory: the flag, then the config
// key, then the default.
func txRecordingDir(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if v := viper.GetString("tx-dir"); v != "" {
		return v
	}
	return txreplay.DefaultDir
}
