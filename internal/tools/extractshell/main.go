// Command extractshell writes the shell code embedded in YAML files to
// separate scripts, so ShellCheck can read it (#546): the run: blocks of a
// composite GitHub Action (action.yml) and the script:, before_script: and
// after_script: lists of GitLab CI files (templates/fhirlint.yml,
// .gitlab-ci.yml). The workflows' own run: blocks are actionlint's job.
//
// Template expressions are replaced by the variable ${TEMPLATE}: the runner
// substitutes a value ShellCheck can't know, and a variable says exactly that,
// where a literal would make it flag every test on it as constant. That covers
// ${{ ... }} in an Action and $[[ ... ]] in a GitLab component.
//
//	go run ./internal/tools/extractshell -o DIR FILE...
//
// Each block becomes DIR/<file>.<n>.sh with a shebang for the shell it runs
// in: bash for an Action step with shell: bash, sh for GitLab.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

func run(args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("extractshell", flag.ContinueOnError)
	flags.SetOutput(stderr)
	out := flags.String("o", "", "directory to write the scripts to")
	if err := flags.Parse(args); err != nil || *out == "" || flags.NArg() == 0 {
		_, _ = fmt.Fprintln(stderr, "usage: extractshell -o DIR FILE...")
		return 2
	}
	if err := os.MkdirAll(*out, 0o750); err != nil {
		_, _ = fmt.Fprintln(stderr, "extractshell:", err)
		return 2
	}
	for _, file := range flags.Args() {
		scripts, err := extractFile(file)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "extractshell: %s: %v\n", file, err)
			return 2
		}
		// No leading dot: a hidden .gitlab-ci.yml.1.sh would slip past *.sh. No
		// colon either: on Windows "C:" would make the name an NTFS stream.
		base := strings.TrimLeft(strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(filepath.ToSlash(file)), ".")
		for i, s := range scripts {
			name := filepath.Join(*out, fmt.Sprintf("%s.%d.sh", base, i+1))
			if err := os.WriteFile(name, []byte(s), 0o600); err != nil {
				_, _ = fmt.Fprintln(stderr, "extractshell:", err)
				return 2
			}
		}
	}
	return 0
}

func extractFile(path string) ([]string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: a path CI passes on the command line
	if err != nil {
		return nil, err
	}
	return extract(path, data)
}

// templateExpr matches the expressions a runner substitutes before the shell
// sees the script.
var templateExpr = regexp.MustCompile(`\$\{\{.*?\}\}|\$\[\[.*?\]\]`)

// extract returns one script per shell block, each with a header that names
// the file and the shell.
func extract(path string, data []byte) ([]string, error) {
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	var scripts []string
	for {
		var doc map[string]any
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if runs, ok := doc["runs"].(map[string]any); ok {
			steps, _ := runs["steps"].([]any)
			for _, s := range steps {
				step, _ := s.(map[string]any)
				body, ok := step["run"].(string)
				if !ok {
					continue
				}
				shell, _ := step["shell"].(string)
				if shell != "bash" && shell != "sh" {
					continue // pwsh, python: not ShellCheck's
				}
				scripts = append(scripts, script(path, shell, body))
			}
			continue
		}
		// GitLab: every top-level mapping with script lists is a job.
		for _, v := range doc {
			job, ok := v.(map[string]any)
			if !ok {
				continue
			}
			for _, key := range []string{"before_script", "script", "after_script"} {
				lines, ok := job[key].([]any)
				if !ok {
					continue
				}
				var b strings.Builder
				for _, l := range lines {
					if s, ok := l.(string); ok {
						b.WriteString(s)
						if !strings.HasSuffix(s, "\n") {
							b.WriteString("\n")
						}
					}
				}
				scripts = append(scripts, script(path, "sh", b.String()))
			}
		}
	}
	return scripts, nil
}

func script(path, shell, body string) string {
	body = templateExpr.ReplaceAllLiteralString(body, "${TEMPLATE}")
	return fmt.Sprintf("#!/usr/bin/env %s\n# extracted from %s by internal/tools/extractshell\n%s", shell, path, body)
}
