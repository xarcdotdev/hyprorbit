package ctl

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"hypr-orbits/internal/module"
	"hypr-orbits/internal/orbit"
)

// PrintOrbit emits orbit information to the configured writer.
func PrintOrbit(w io.Writer, opts Options, record *orbit.Record) error {
	if opts.Quiet {
		return nil
	}
	if opts.JSON {
		return encodeJSON(w, record)
	}
	if record == nil {
		return fmt.Errorf("orbit: nothing to print")
	}
	parts := []string{record.Name}
	if record.Label != "" {
		parts = append(parts, record.Label)
	}
	if record.Color != "" {
		parts = append(parts, record.Color)
	}
	_, err := fmt.Fprintln(w, strings.Join(parts, "\t"))
	return err
}

// PrintModule emits module results to stdout respecting JSON/quiet flags.
func PrintModule(w io.Writer, opts Options, result *module.Result) error {
	if opts.Quiet {
		return nil
	}
	if opts.JSON {
		return encodeJSON(w, result)
	}
	if result == nil {
		return fmt.Errorf("module: nothing to print")
	}
	parts := []string{result.Action, result.Workspace}
	if result.Orbit != "" {
		parts = append(parts, result.Orbit)
	}
	_, err := fmt.Fprintln(w, strings.Join(parts, "\t"))
	return err
}

// PrintModuleList prints a slice of module results.
func PrintModuleList(w io.Writer, opts Options, results []*module.Result) error {
	if opts.Quiet {
		return nil
	}
	if opts.JSON {
		if results == nil {
			results = []*module.Result{}
		}
		return encodeJSON(w, results)
	}
	for _, res := range results {
		if err := PrintModule(w, Options{JSON: false, Quiet: false}, res); err != nil {
			return err
		}
	}
	return nil
}

func encodeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
