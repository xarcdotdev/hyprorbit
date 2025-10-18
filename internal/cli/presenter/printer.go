package presenter

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
	plainOpts := Options{JSON: false, Quiet: false, NoColor: opts.NoColor}
	for _, res := range results {
		if err := PrintModule(w, plainOpts, res); err != nil {
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

// PrintWindowMoves renders window move results, handling single or batched responses.
func PrintWindowMoves(w io.Writer, opts Options, results []WindowMoveResult) error {
	if opts.Quiet {
		return nil
	}
	if opts.JSON {
		if len(results) == 1 {
			return encodeJSON(w, &results[0])
		}
		return encodeJSON(w, results)
	}
	if len(results) == 0 {
		return fmt.Errorf("window: nothing to print")
	}
	for _, result := range results {
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
		if result.Temporary {
			annotations = append(annotations, "temp")
		}
		if len(annotations) > 0 {
			parts = append(parts, "["+strings.Join(annotations, ", ")+"]")
		}
		if _, err := fmt.Fprintln(w, strings.Join(parts, "\t")); err != nil {
			return err
		}
	}
	return nil
}

const (
	// Limits keep the table readable while JSON output remains full-length.
	windowListMaxClass = 40
	windowListMaxTitle = 70
)

// PrintWindowList renders window metadata in a tabular view.
func PrintWindowList(w io.Writer, opts Options, windows []WindowSummary) error {
	if opts.Quiet {
		return nil
	}
	if opts.JSON {
		if windows == nil {
			windows = []WindowSummary{}
		}
		return encodeJSON(w, windows)
	}
	headers := []string{"WORKSPACE", "MODULE", "ORBIT", "ADDRESS", "CLASS", "TITLE"}
	rows := make([][]string, len(windows))
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = runeLen(header)
	}

	for i, window := range windows {
		workspace := dashIfEmpty(window.Workspace)
		module := dashIfEmpty(window.Module)
		orbit := dashIfEmpty(window.Orbit)
		address := dashIfEmpty(window.Address)
		class := dashIfEmpty(window.Class)
		title := dashIfEmpty(window.Title)
		if class != "-" {
			class = truncateMiddle(class, windowListMaxClass)
		}
		if title != "-" {
			title = truncateMiddle(title, windowListMaxTitle)
		}

		row := []string{workspace, module, orbit, address, class, title}
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
		if row[1] != "-" {
			colors[1] = colorOrEmpty(opts, ansiCyan)
		}
		if row[2] != "-" {
			colors[2] = colorOrEmpty(opts, ansiCyan)
		}
		if row[3] != "-" {
			colors[3] = colorOrEmpty(opts, ansiGrey)
		}
		if err := printTableRow(w, row, widths, colors, reset, false); err != nil {
			return err
		}
	}
	return nil
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
	headers := []string{"NAME", "STATUS", "ACTIVE_MODULE", "WINDOWS"}
	rows := make([][]string, len(summaries))
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = runeLen(header)
	}

	for i, summary := range summaries {
		status := dashIfEmpty(summary.Status)
		activeModule := dashIfEmpty(summary.ActiveModule)
		windows := fmt.Sprintf("%d", summary.Windows)
		row := []string{summary.Name, status, activeModule, windows}
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
		case "focused":
			colors[1] = colorOrEmpty(opts, ansiGreen)
		case "sleeping":
			colors[1] = colorOrEmpty(opts, ansiGrey)
		default:
			colors[1] = colorOrEmpty(opts, ansiYellow)
		}
		if row[2] != "-" {
			colors[2] = colorOrEmpty(opts, ansiCyan)
		}
		colors[3] = colorOrEmpty(opts, ansiGrey)
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

func truncateMiddle(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	keep := limit - 3
	head := keep / 2
	tail := keep - head
	return string(runes[:head]) + "..." + string(runes[len(runes)-tail:])
}
