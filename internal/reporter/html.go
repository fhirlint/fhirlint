package reporter

import (
	"html/template"
	"os"
	"time"

	"github.com/fhirlint/fhirlint/internal/validator"
)

const htmlTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>fhirlint Report</title>
<style>
  body { font-family: system-ui, sans-serif; max-width: 960px; margin: 2rem auto; padding: 0 1rem; color: #1a1a1a; }
  h1 { font-size: 1.5rem; margin-bottom: 0.25rem; }
  .meta { color: #666; font-size: 0.875rem; margin-bottom: 2rem; }
  .summary { display: flex; gap: 1.5rem; padding: 1rem; background: #f5f5f5; border-radius: 8px; margin-bottom: 2rem; }
  .stat { text-align: center; }
  .stat-value { font-size: 1.75rem; font-weight: bold; }
  .stat-label { font-size: 0.75rem; color: #666; text-transform: uppercase; }
  .valid .stat-value { color: #16a34a; }
  .errors .stat-value { color: #dc2626; }
  .warnings .stat-value { color: #d97706; }
  .file { margin-bottom: 1.5rem; border: 1px solid #e5e5e5; border-radius: 8px; overflow: hidden; }
  .file-header { padding: 0.75rem 1rem; background: #f9f9f9; font-weight: bold; font-size: 0.875rem; border-bottom: 1px solid #e5e5e5; }
  .file-valid .file-header { border-left: 4px solid #16a34a; }
  .file-invalid .file-header { border-left: 4px solid #dc2626; }
  .issue { padding: 0.6rem 1rem; border-bottom: 1px solid #f0f0f0; font-size: 0.875rem; }
  .issue:last-child { border-bottom: none; }
  .badge { display: inline-block; padding: 0.1rem 0.5rem; border-radius: 4px; font-size: 0.75rem; font-weight: bold; margin-right: 0.5rem; }
  .badge-error { background: #fee2e2; color: #dc2626; }
  .badge-warning { background: #fef3c7; color: #d97706; }
  .badge-information { background: #dbeafe; color: #2563eb; }
  .location { color: #888; font-size: 0.8rem; margin-top: 0.2rem; }
  .ok-msg { padding: 0.75rem 1rem; color: #16a34a; font-size: 0.875rem; }
</style>
</head>
<body>
<h1>fhirlint Validation Report</h1>
<p class="meta">Generated {{ .Generated }} · FHIR {{ .FHIRVersion }}</p>

<div class="summary">
  <div class="stat"><div class="stat-value">{{ .Summary.Total }}</div><div class="stat-label">Issues</div></div>
  <div class="stat errors"><div class="stat-value">{{ .Summary.Errors }}</div><div class="stat-label">Errors</div></div>
  <div class="stat warnings"><div class="stat-value">{{ .Summary.Warnings }}</div><div class="stat-label">Warnings</div></div>
  <div class="stat valid"><div class="stat-value">{{ .ValidCount }}/{{ len .Files }}</div><div class="stat-label">Valid</div></div>
</div>

{{ range .Files }}
<div class="file {{ if .Valid }}file-valid{{ else }}file-invalid{{ end }}">
  <div class="file-header">{{ .Label }}{{ if .Valid }} ✓{{ end }}</div>
  {{ if .Valid }}
  <div class="ok-msg">No issues found.</div>
  {{ else }}
  {{ range .Issues }}
  <div class="issue">
    <span class="badge badge-{{ .Severity }}">{{ .Severity }}</span>{{ .Message }}
    {{ if .Location }}<div class="location">@ {{ .Location }}</div>{{ end }}
  </div>
  {{ end }}
  {{ end }}
</div>
{{ end }}
</body>
</html>`

type htmlData struct {
	Generated   string
	FHIRVersion string
	Files       []*validator.Result
	Summary     JSONSummary
	ValidCount  int
}

func HTML(results []*validator.Result, minSeverity, fhirVersion, dest string) error {
	report := buildJSONReport(results, minSeverity)

	validCount := 0
	for _, r := range results {
		if r.Valid {
			validCount++
		}
	}

	data := htmlData{
		Generated:   time.Now().Format("2006-01-02 15:04:05"),
		FHIRVersion: fhirVersion,
		Files:       report.Files,
		Summary:     report.Summary,
		ValidCount:  validCount,
	}

	tmpl, err := template.New("report").Parse(htmlTmpl)
	if err != nil {
		return err
	}

	if dest == "" {
		return tmpl.Execute(os.Stdout, data)
	}
	f, err := os.Create(dest) //nolint:gosec // intentional: user-supplied output path
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return tmpl.Execute(f, data)
}
