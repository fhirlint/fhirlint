package qualify

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
)

// Terminal writes a concise human-readable qualification summary to w.
func Terminal(w io.Writer, r *Report) {
	fmt.Fprintln(w, "fhirlint Computer System Validation — Operational Qualification")
	fmt.Fprintf(w, "  Tool version:  %s\n", r.ToolVersion)
	fmt.Fprintf(w, "  JAR version:   %s\n", r.JARVersion)
	fmt.Fprintf(w, "  JAR SHA256:    %s\n", r.JARSHA256)
	fmt.Fprintf(w, "  FHIR version:  %s\n", r.FHIRVersion)
	fmt.Fprintf(w, "  Terminology:   %s\n", r.Terminology)
	fmt.Fprintf(w, "  Timestamp:     %s\n\n", r.Timestamp)

	fmt.Fprintf(w, "Test cases: %d passed · %d failed\n\n", r.Passed, r.Failed)

	width := 0
	for _, c := range r.Cases {
		if len(c.Name) > width {
			width = len(c.Name)
		}
	}
	for _, c := range r.Cases {
		status := "PASS"
		if !c.Pass {
			status = "FAIL"
		}
		fmt.Fprintf(w, "  %s  %-*s  → %s\n", status, width, c.Name, c.Detail)
	}

	fmt.Fprintln(w)
	if r.Qualified {
		fmt.Fprintln(w, "Result: QUALIFIED ✓")
	} else {
		fmt.Fprintln(w, "Result: NOT QUALIFIED ✗")
	}
}

// JSON writes the report as indented JSON. Empty dest writes to stdout.
func JSON(r *Report, dest string) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if dest == "" {
		_, err := os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(dest, data, 0600)
}

// HTML writes a self-contained, print-friendly qualification report. Empty dest
// writes to stdout. The document can be printed to PDF from any browser for QMS
// or Design History File attachment.
func HTML(r *Report, dest string) error {
	tmpl, err := template.New("qualify").Parse(qualifyHTMLTmpl)
	if err != nil {
		return err
	}
	if dest == "" {
		return tmpl.Execute(os.Stdout, r)
	}
	f, err := os.Create(dest) //nolint:gosec // user-supplied output path
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return tmpl.Execute(f, r)
}

const qualifyHTMLTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>fhirlint Operational Qualification Report</title>
<style>
  body { font-family: system-ui, sans-serif; max-width: 900px; margin: 2rem auto; padding: 0 1rem; color: #1a1a1a; }
  h1 { font-size: 1.4rem; margin-bottom: 0.25rem; }
  .sub { color: #555; margin-bottom: 1.5rem; }
  table { border-collapse: collapse; width: 100%; margin: 1rem 0; font-size: 0.9rem; }
  th, td { text-align: left; padding: 0.5rem 0.6rem; border-bottom: 1px solid #e2e2e2; vertical-align: top; }
  th { background: #f6f6f6; }
  .meta td:first-child { color: #555; width: 12rem; }
  .mono { font-family: ui-monospace, monospace; font-size: 0.82rem; word-break: break-all; }
  .pass { color: #137333; font-weight: 600; }
  .fail { color: #c5221f; font-weight: 600; }
  .banner { display: inline-block; padding: 0.5rem 1rem; border-radius: 6px; font-weight: 700; font-size: 1.1rem; margin: 0.5rem 0 1.5rem; }
  .qualified { background: #e6f4ea; color: #137333; }
  .notqualified { background: #fce8e6; color: #c5221f; }
  .signoff { margin-top: 2.5rem; border-top: 1px solid #ccc; padding-top: 1rem; color: #555; font-size: 0.9rem; }
  .signoff div { margin: 1.5rem 0; }
  @media print { body { margin: 0; } }
</style>
</head>
<body>
<h1>fhirlint — Computer System Validation</h1>
<div class="sub">Operational Qualification (OQ) Report</div>

{{if .Qualified}}<div class="banner qualified">QUALIFIED ✓</div>{{else}}<div class="banner notqualified">NOT QUALIFIED ✗</div>{{end}}

<table class="meta">
  <tr><td>Tool version</td><td class="mono">{{.ToolVersion}}</td></tr>
  <tr><td>Validator JAR version</td><td class="mono">{{.JARVersion}}</td></tr>
  <tr><td>Validator JAR SHA256</td><td class="mono">{{.JARSHA256}}</td></tr>
  <tr><td>FHIR version</td><td class="mono">{{.FHIRVersion}}</td></tr>
  <tr><td>Terminology</td><td class="mono">{{.Terminology}}</td></tr>
  <tr><td>Generated</td><td class="mono">{{.Timestamp}}</td></tr>
  <tr><td>Result</td><td>{{.Passed}} passed, {{.Failed}} failed</td></tr>
</table>

<table>
  <thead><tr><th>Result</th><th>Test case</th><th>Expectation</th><th>Outcome</th></tr></thead>
  <tbody>
  {{range .Cases}}
    <tr>
      <td class="{{if .Pass}}pass{{else}}fail{{end}}">{{if .Pass}}PASS{{else}}FAIL{{end}}</td>
      <td class="mono">{{.Name}}<br><span style="color:#777">{{.Description}}</span></td>
      <td>{{.Expectation}}</td>
      <td>{{.Detail}}</td>
    </tr>
  {{end}}
  </tbody>
</table>

<div class="signoff">
  This report documents the operational qualification of fhirlint against a defined
  set of known-valid and known-invalid FHIR resources. Print to PDF for attachment to
  a Design History File or QMS record.
  <div>Reviewed by: ________________________________&nbsp;&nbsp;&nbsp;Date: ______________</div>
  <div>Approved by: ________________________________&nbsp;&nbsp;&nbsp;Date: ______________</div>
</div>
</body>
</html>
`
