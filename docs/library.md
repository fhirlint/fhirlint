# Go library

fhirlint as a Go library.

## Go library

fhirlint can be embedded as a Go library via `pkg/fhirlint`:

```bash
go get github.com/fhirlint/fhirlint/pkg/fhirlint
```

```go
import "github.com/fhirlint/fhirlint/pkg/fhirlint"

// Validate from bytes (JSON or XML detected automatically)
result, err := fhirlint.Validate(patientJSON, fhirlint.Options{
    FHIRVersion:         "4.0.1",
    NoTerminologyServer: true,
})

// Validate a single file
result, err := fhirlint.ValidateFile("./patient.json", fhirlint.Options{
    FHIRVersion: "4.0.1",
    IGs:         []string{"kbv.basis#1.5.0"},
})

// Validate all files in a directory (single JVM invocation)
results, err := fhirlint.ValidateDir("./fhir/", fhirlint.Options{
    FHIRVersion: "4.0.1",
})

// Validate from an HTTP endpoint
result, err := fhirlint.ValidateURL("https://hapi.fhir.org/baseR4/Patient/1", fhirlint.Options{
    FHIRVersion: "4.0.1",
})
```

The library requires Java 17+ and downloads the validator JAR on first use (~250 MB). Set `JARPath` or `FHIRLINT_JAR` to provide a pre-downloaded JAR.
