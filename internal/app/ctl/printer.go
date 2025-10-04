package ctl

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"hyprorbit/internal/module"
	"hyprorbit/internal/orbit"
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

// PrintModuleStatus prints the current module/orbit association.
func PrintModuleStatus(w io.Writer, opts Options, status *module.Status) error {
	if opts.Quiet {
		return nil
	}
	if opts.JSON {
		return encodeJSON(w, status)
	}
	if status == nil {
		return fmt.Errorf("module: nothing to print")
	}
	parts := []string{status.Module, status.Workspace}
	if status.Orbit.Name != "" {
		parts = append(parts, status.Orbit.Name)
	}
	if status.Orbit.Label != "" {
		parts = append(parts, status.Orbit.Label)
	}
	if status.Orbit.Color != "" {
		parts = append(parts, status.Orbit.Color)
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
		if err := PrintModule(w, Options{JSON: false, Quiet: false, NoColor: opts.NoColor}, res); err != nil {
			return err
		}
	}
	return nil
}

// PrintWorkspaceSummaries emits module workspace summaries.
func PrintWorkspaceSummaries(w io.Writer, opts Options, summaries []module.WorkspaceSummary) error {
	if opts.Quiet {
		return nil
	}
	if opts.JSON {
		if summaries == nil {
			summaries = []module.WorkspaceSummary{}
		}
		return encodeJSON(w, summaries)
	}
	headers := []string{"NAME", "STATUS", "MODULE", "ORBIT", "MONITOR", "WINDOWS"}
	// Prepare table data and track maximum widths per column.
	rows := make([][]string, len(summaries))
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = runeLen(header)
	}

	for i, summary := range summaries {
		status := "special"
		switch {
		case summary.Temporary:
			status = "temp"
		case summary.Configured:
			if summary.Exists {
				status = "active"
			} else {
				status = "inactive"
			}
		case summary.Special:
			status = "special"
		case summary.Exists:
			status = "custom"
		}
		moduleName := dashIfEmpty(summary.Module)
		orbitName := dashIfEmpty(summary.Orbit)
		monitor := dashIfEmpty(summary.Monitor)
		windows := "-"
		if summary.Exists {
			windows = fmt.Sprintf("%d", summary.Windows)
		}

		row := []string{summary.Name, status, moduleName, orbitName, monitor, windows}
		rows[i] = row
		for col := range row {
			if l := runeLen(row[col]); l > widths[col] {
				widths[col] = l
			}
		}
	}

	// Render header.
	reset := colorOrEmpty(opts, ansiReset)
	if err := printTableRow(w, headers, widths, nil, reset, true); err != nil {
		return err
	}

	for _, row := range rows {
		colors := make([]string, len(row))
		switch row[1] {
		case "active":
			colors[1] = colorOrEmpty(opts, ansiGreen)
		case "inactive":
			colors[1] = colorOrEmpty(opts, ansiGrey)
		default:
			colors[1] = colorOrEmpty(opts, ansiYellow)
		}
		if row[2] != "-" {
			colors[2] = colorOrEmpty(opts, ansiCyan)
		}
		if row[4] != "-" {
			colors[4] = colorOrEmpty(opts, ansiWhite)
		}
		if err := printTableRow(w, row, widths, colors, reset, true); err != nil {
			return err
		}
	}
	return nil
}

// PrintWindowMove renders the result of a window move operation.
func PrintWindowMove(w io.Writer, opts Options, result *WindowMoveResult) error {
	if opts.Quiet {
		return nil
	}
	if opts.JSON {
		return encodeJSON(w, result)
	}
	if result == nil {
		return fmt.Errorf("window: nothing to print")
	}
	parts := []string{dashIfEmpty(result.Window), dashIfEmpty(result.Workspace)}
	if result.Module != "" {
		parts = append(parts, result.Module)
	}
	if result.Orbit != "" {
		parts = append(parts, result.Orbit)
	}
	annotations := make([]string, 0, 2)
	if result.Created {
		annotations = append(annotations, "created")
	}
	if result.Focused {
		annotations = append(annotations, "focused")
	}
	if len(annotations) > 0 {
		parts = append(parts, "["+strings.Join(annotations, ", ")+"]")
	}
	_, err := fmt.Fprintln(w, strings.Join(parts, "\t"))
	return err
}

// PrintOrbitSummaries emits orbit information with runtime status details.
func PrintOrbitSummaries(w io.Writer, opts Options, summaries []orbit.Summary) error {
	if opts.Quiet {
		return nil
	}
	if opts.JSON {
		if summaries == nil {
			summaries = []orbit.Summary{}
		}
		return encodeJSON(w, summaries)
	}
	headers := []string{"NAME", "STATUS", "ACTIVE_MODULE"}
	rows := make([][]string, len(summaries))
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = runeLen(header)
	}

	for i, summary := range summaries {
		status := dashIfEmpty(summary.Status)
		activeModule := dashIfEmpty(summary.ActiveModule)
		row := []string{summary.Name, status, activeModule}
		rows[i] = row
		for col, value := range row {
			if l := runeLen(value); l > widths[col] {
				widths[col] = l
			}
		}
	}

	reset := colorOrEmpty(opts, ansiReset)
	if err := printTableRow(w, headers, widths, nil, reset, false); err != nil {
		return err
	}

	for _, row := range rows {
		colors := make([]string, len(row))
		switch strings.ToLower(row[1]) {
		case "active":
			colors[1] = colorOrEmpty(opts, ansiGreen)
		case "inactive":
			colors[1] = colorOrEmpty(opts, ansiGrey)
		default:
			colors[1] = colorOrEmpty(opts, ansiYellow)
		}
		if row[2] != "-" {
			colors[2] = colorOrEmpty(opts, ansiCyan)
		}
		if err := printTableRow(w, row, widths, colors, reset, false); err != nil {
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

const (
	ansiReset  = "\033[0m"
	ansiGreen  = "\033[32m"
	ansiGrey   = "\033[90m"
	ansiCyan   = "\033[36m"
	ansiWhite  = "\033[97m"
	ansiYellow = "\033[33m"
)

func printTableRow(w io.Writer, columns []string, widths []int, colors []string, reset string, alignLastRight bool) error {
	for i, text := range columns {
		color := ""
		if colors != nil {
			color = colors[i]
		}
		var err error
		if i == len(columns)-1 {
			if alignLastRight {
				if color != "" {
					_, err = fmt.Fprintf(w, "%s%*s%s", color, widths[i], text, reset)
				} else {
					_, err = fmt.Fprintf(w, "%*s", widths[i], text)
				}
			} else {
				if color != "" {
					_, err = fmt.Fprintf(w, "%s%-*s%s", color, widths[i], text, reset)
				} else {
					_, err = fmt.Fprintf(w, "%-*s", widths[i], text)
				}
			}
		} else {
			if color != "" {
				_, err = fmt.Fprintf(w, "%s%-*s%s", color, widths[i], text, reset)
			} else {
				_, err = fmt.Fprintf(w, "%-*s", widths[i], text)
			}
		}
		if err != nil {
			return err
		}
		if i != len(columns)-1 {
			if _, err := fmt.Fprint(w, "  "); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprintln(w)
	return err
}

func dashIfEmpty(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func runeLen(value string) int {
	return utf8.RuneCountInString(value)
}

func colorOrEmpty(opts Options, code string) string {
	if opts.NoColor {
		return ""
	}
	return code
}
