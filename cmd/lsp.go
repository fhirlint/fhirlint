package cmd

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/fhirlint/fhirlint/internal/lsp"
	"github.com/fhirlint/fhirlint/internal/profiles"
	"github.com/fhirlint/fhirlint/internal/validator"
)

var (
	flagLSPServer      string
	flagLSPFHIRVersion string
	flagLSPIG          []string
	flagLSPProfile     []string
	flagLSPNoSuppress  bool
)

var lspCmd = &cobra.Command{
	Use:   "lsp",
	Short: "Run a language server so findings appear inline in your editor",
	Long: `Run a Language Server Protocol server over stdio, so validation findings show
up as you edit instead of only when you run the CLI.

A validator server is started and kept warm for the session, so validating a
buffer takes milliseconds rather than the tens of seconds a cold JVM needs.
Point it at an already-running one with --server to share it across editors.

Diagnostics carry the HL7 message id as the diagnostic code, hovering explains
it offline, and a quick fix writes a suppression into fhirlint.yml.

This command speaks LSP on stdin/stdout and is meant to be launched by an
editor, not run by hand. See docs/lsp.md for editor configuration.`,
	Args:         cobra.NoArgs,
	RunE:         runLSP,
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(lspCmd)

	lspCmd.Flags().StringVar(&flagLSPServer, "server", "",
		"Use an already-running validator server (e.g. http://localhost:8080) instead of starting one")
	lspCmd.Flags().StringVar(&flagLSPFHIRVersion, "fhir-version", defaultFHIRVersion,
		"FHIR version to validate against ("+validator.FHIRVersionList()+")")
	lspCmd.Flags().StringArrayVar(&flagLSPIG, "ig", nil,
		"IG package (or alias) to load, e.g. hl7.fhir.us.core#9.0.0 (repeatable)")
	lspCmd.Flags().StringArrayVarP(&flagLSPProfile, "profile", "p", nil,
		"Profile alias or URL applied to every validated document (repeatable)")
	lspCmd.Flags().BoolVar(&flagLSPNoSuppress, "no-suppress-action", false,
		"Do not offer the quick fix that writes suppressions into fhirlint.yml")
}

func runLSP(cmd *cobra.Command, _ []string) error {
	// Diagnostics go to the client over stdout, so every human-readable line
	// has to go to stderr or it corrupts the protocol stream.
	logW := os.Stderr

	if !cmd.Flags().Changed("fhir-version") && viper.IsSet("fhir-version") {
		flagLSPFHIRVersion = viper.GetString("fhir-version")
	}
	if !cmd.Flags().Changed("ig") && viper.IsSet("ig") {
		flagLSPIG = viper.GetStringSlice("ig")
	}
	if !cmd.Flags().Changed("profile") && viper.IsSet("profile") {
		flagLSPProfile = viper.GetStringSlice("profile")
	}

	resolvedProfiles := make([]string, 0, len(flagLSPProfile))
	for _, p := range flagLSPProfile {
		resolvedProfiles = append(resolvedProfiles, profiles.Resolve(p))
	}
	opts := validator.Options{Profiles: resolvedProfiles}

	serverURL := flagLSPServer
	if serverURL == "" {
		if err := validator.CheckJava(); err != nil {
			return err
		}
		port, err := freePort()
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(logW, "fhirlint lsp: starting validator server…")
		srv, err := validator.StartServer(validator.ServerConfig{
			Port:             port,
			FHIRVersion:      flagLSPFHIRVersion,
			IGs:              resolveIGs(flagLSPIG),
			JARPath:          viper.GetString("jar"),
			ValidatorVersion: viper.GetString("validator-version"),
			Proxy:            validator.ProxyConfig{Proxy: viper.GetString("proxy"), HTTPSProxy: viper.GetString("https-proxy")},
		}, logW, serverStartupTimeout)
		if err != nil {
			return err
		}
		defer func() { _ = srv.Stop() }()
		serverURL = srv.URL()
	}
	_, _ = fmt.Fprintf(logW, "fhirlint lsp: ready (validator at %s)\n", serverURL)

	var suppressor lsp.Suppressor
	if !flagLSPNoSuppress {
		suppressor = &configSuppressor{path: "fhirlint.yml"}
	}

	server := lsp.NewServer(os.Stdin, os.Stdout,
		&warmBackend{url: serverURL, opts: opts}, suppressor, logW, version)
	return server.Run()
}

// freePort asks the kernel for an unused port. The validator server needs a
// concrete port number, and an editor may run several language servers at once,
// so a fixed default would collide. The listener is closed before the server
// binds, which leaves a small race — acceptable next to the alternative of
// picking a number and hoping.
func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("finding a free port for the validator server: %w", err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// warmBackend validates editor buffers against a warm validator server.
type warmBackend struct {
	url  string
	opts validator.Options
}

func (b *warmBackend) ValidateContent(content []byte, label string) (*validator.Result, error) {
	opts := b.opts
	if opts.Timeout == 0 {
		// An editor loop wants to fail fast rather than hang on a wedged server.
		opts.Timeout = 60 * time.Second
	}
	return validator.ValidateBytesViaServer(b.url, content, label, opts)
}

// configSuppressor appends a suppression to fhirlint.yml.
type configSuppressor struct {
	path string
}

func (c *configSuppressor) Suppress(messageID string) error {
	existing, err := os.ReadFile(c.path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", c.path, err)
	}
	updated, err := addSuppression(string(existing), messageID)
	if err != nil {
		return err
	}
	if updated == "" {
		return nil // already present
	}
	// The path is fixed at construction (fhirlint.yml in the working
	// directory), never taken from a protocol message.
	if err := os.WriteFile(c.path, []byte(updated), 0o600); err != nil { //nolint:gosec // G703: path is not request-controlled
		return fmt.Errorf("writing %s: %w", c.path, err)
	}
	return nil
}

// addSuppression inserts a suppression rule into a fhirlint.yml document,
// returning the new content or "" when the rule is already there.
//
// This edits the file as text rather than round-tripping it through a YAML
// encoder on purpose: a config carries comments explaining why each suppression
// exists, and re-encoding would delete them. Duplicating the `suppress:` key
// would produce an invalid document, so an existing key is extended in place.
func addSuppression(content, messageID string) (string, error) {
	if strings.TrimSpace(messageID) == "" {
		return "", fmt.Errorf("empty message id")
	}
	entry := "messageId:" + messageID
	lines := strings.Split(content, "\n")

	suppressAt := -1
	for i, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		if trimmed == "suppress:" {
			suppressAt = i
			continue
		}
		// Only treat it as present when it is an active list item, not a
		// commented-out example.
		if suppressAt >= 0 && isSuppressEntry(line, entry) {
			return "", nil
		}
	}

	if suppressAt < 0 {
		var b strings.Builder
		b.WriteString(content)
		if content != "" && !strings.HasSuffix(content, "\n") {
			b.WriteString("\n")
		}
		if content != "" {
			b.WriteString("\n")
		}
		b.WriteString("suppress:\n  - " + entry + "\n")
		return b.String(), nil
	}

	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:suppressAt+1]...)
	out = append(out, "  - "+entry)
	out = append(out, lines[suppressAt+1:]...)
	return strings.Join(out, "\n"), nil
}

// isSuppressEntry reports whether a line is an active list item for entry.
func isSuppressEntry(line, entry string) bool {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "-") {
		return false
	}
	t = strings.TrimSpace(strings.TrimPrefix(t, "-"))
	t = strings.Trim(t, `"'`)
	return t == entry
}
