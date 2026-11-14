// Package scaling holds the linear scaling test and the benchmarks of
// fhirlint's Go-side pipeline after the validator: suppression, baseline,
// package hints, --group, every report format and --slowest (#552). It has no
// runtime code. The output parsers are covered in internal/validator, where
// they are visible.
package scaling
