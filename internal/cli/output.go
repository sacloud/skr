// Copyright 2022-2026 The sacloud/skr Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/alecthomas/kong"
	"github.com/goccy/go-yaml"
	"github.com/sacloud/sacloud-sdk-go/common/saclient"
	"golang.org/x/term"
	"golang.org/x/text/width"
)

const defaultOutputType = "json"

func writeOutputWithFormat(ctx *kong.Context, format string, value any, zones ...[]string) error {
	if query, set, err := queryExpression(ctx); err != nil {
		return err
	} else if set {
		return writeQueryOutput(ctx.Stdout, query, value)
	}

	switch format {
	case "json":
		encoder := json.NewEncoder(ctx.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	case "table":
		var zoneNames []string
		if len(zones) > 0 {
			zoneNames = zones[0]
		}
		return writeTable(ctx.Stdout, value, zoneNames)
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

func outputType(ctx *kong.Context) (string, error) {
	query, set, err := queryExpression(ctx)
	if err != nil {
		return "", err
	}
	if set {
		if _, err := compileQuery(query); err != nil {
			return "", err
		}
		return "json", nil
	}

	for _, flag := range ctx.Flags() {
		if flag.Name == "output" && flag.Set {
			value, ok := ctx.FlagValue(flag).(*string)
			if !ok || value == nil {
				return "", fmt.Errorf("read --output value: unexpected value type")
			}
			return validateOutputType(*value)
		}
	}

	format, err := profileOutputType(os.Environ())
	if err != nil {
		return "", err
	}
	return validateOutputType(format)
}

func validateOutputType(format string) (string, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "json", "table":
		return format, nil
	default:
		return "", fmt.Errorf("unsupported output format %q (choose json or table)", format)
	}
}

func profileOutputType(env []string) (string, error) {
	profileOp, err := saclient.NewProfileOp(env)
	if err != nil {
		return "", fmt.Errorf("locate profile for output format: %w", err)
	}

	profileName := "default"
	currentPath := filepath.Join(profileOp.Dir(), "current")
	//nolint:gosec // The profile directory is resolved by saclient from standard profile environment variables.
	current, err := os.ReadFile(currentPath)
	if err == nil {
		if name := strings.TrimSpace(string(current)); name != "" {
			if filepath.IsAbs(name) || filepath.Base(name) != name || name == "." || name == ".." {
				return "", fmt.Errorf("invalid current profile name %q", name)
			}
			profileName = name
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read current profile for output format: %w", err)
	}

	profileDir := filepath.Join(profileOp.Dir(), profileName)
	for _, filename := range []string{"config.yaml", "config.json"} {
		path := filepath.Join(profileDir, filename)
		//nolint:gosec // The profile name is validated and the filename is fixed.
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("read profile output format from %s: %w", path, err)
		}
		var attributes map[string]any
		if err := yaml.Unmarshal(data, &attributes); err != nil {
			return "", fmt.Errorf("decode profile output format from %s: %w", path, err)
		}
		format, ok, err := configuredOutputType(attributes)
		if err != nil {
			return "", fmt.Errorf("read output format from %s: %w", path, err)
		}
		if ok {
			return format, nil
		}
	}

	return defaultOutputType, nil
}

func configuredOutputType(attributes map[string]any) (string, bool, error) {
	if cliConfig, ok := mapValue(attributes, "cli"); ok {
		cliAttributes, ok := cliConfig.(map[string]any)
		if !ok {
			return "", false, fmt.Errorf("cli must be an object")
		}
		if value, exists := mapValue(cliAttributes, "default_output_type", "DefaultOutputType"); exists {
			format, ok := value.(string)
			if !ok {
				return "", false, fmt.Errorf("cli.default_output_type must be a string")
			}
			return format, true, nil
		}
	}
	if value, ok := mapValue(attributes, "DefaultOutputType", "default_output_type"); ok {
		format, ok := value.(string)
		if !ok {
			return "", false, fmt.Errorf("DefaultOutputType must be a string")
		}
		return format, true, nil
	}
	return defaultOutputType, false, nil
}

func mapValue(values map[string]any, keys ...string) (any, bool) {
	for key, value := range values {
		for _, candidate := range keys {
			if strings.EqualFold(key, candidate) {
				return value, true
			}
		}
	}
	return nil, false
}

func writeTable(writer io.Writer, value any, zones []string) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode output for table: %w", err)
	}
	var decoded any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("decode output for table: %w", err)
	}

	var rows []map[string]any
	switch result := decoded.(type) {
	case []any:
		if len(result) == 0 {
			return writeEmptyTable(writer, terminalWidth(writer))
		}
		rows = make([]map[string]any, 0, len(result))
		for i, item := range result {
			row, ok := item.(map[string]any)
			if !ok {
				return fmt.Errorf("table output row %d is not an object", i)
			}
			rows = append(rows, row)
		}
	case map[string]any:
		rows = []map[string]any{result}
	default:
		return fmt.Errorf("table output requires an object or an array of objects")
	}
	if len(zones) > 0 && len(zones) != len(rows) {
		return fmt.Errorf("table output has %d zone labels for %d rows", len(zones), len(rows))
	}

	fields := make(map[string]struct{})
	for _, row := range rows {
		for field := range row {
			fields[field] = struct{}{}
		}
	}
	columns := orderedTableColumns(fields, len(zones) > 0)
	allRows := make([][]string, len(rows))
	for i, row := range rows {
		allRows[i] = make([]string, len(columns))
		for j, column := range columns {
			if column == "Zone" && len(zones) > 0 {
				allRows[i][j] = zones[i]
				continue
			}
			allRows[i][j] = tableCellValue(column, row[column])
		}
	}

	width := terminalWidth(writer)
	columns, allRows, widths, omitted := fitTable(columns, allRows, width)
	if err := writeTableBorder(writer, widths); err != nil {
		return err
	}
	if err := writeTableRow(writer, columns, widths); err != nil {
		return err
	}
	if err := writeTableBorder(writer, widths); err != nil {
		return err
	}
	for _, row := range allRows {
		if err := writeTableRow(writer, row, widths); err != nil {
			return err
		}
	}
	if err := writeTableBorder(writer, widths); err != nil {
		return err
	}
	if omitted > 0 {
		message := truncateTableCell(fmt.Sprintf("... +%d columns omitted", omitted), width)
		if _, err := fmt.Fprintln(writer, message); err != nil {
			return err
		}
	}
	return nil
}

func terminalWidth(writer io.Writer) int {
	if file, ok := writer.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		width, _, err := term.GetSize(int(file.Fd()))
		if err == nil && width > 0 {
			return width
		}
	}
	if columns, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && columns > 0 {
		return columns
	}
	return 100
}

func fitTable(columns []string, rows [][]string, terminalWidth int) ([]string, [][]string, []int, int) {
	selected := make([]string, 0, len(columns))
	widths := make([]int, 0, len(columns))
	cellBudget := terminalWidth - 1
	for _, column := range columns {
		minimum := max(minTableColumnWidth(column), displayWidth(column))
		nextCount := len(selected) + 1
		if len(selected) > 0 && nextCount*3+cellBudgetFor(widths)+minimum > cellBudget {
			break
		}
		selected = append(selected, column)
		widths = append(widths, minimum)
	}
	if len(selected) == 0 && len(columns) > 0 {
		selected = append(selected, columns[0])
		widths = append(widths, max(1, terminalWidth-4))
	}

	cellBudget = max(1, terminalWidth-3*len(selected)-1)
	used := 0
	for _, columnWidth := range widths {
		used += columnWidth
	}
	remaining := max(0, cellBudget-used)
	for i, column := range selected {
		desired := min(maxTableColumnWidth(column), max(displayWidth(column), widths[i]))
		for _, row := range rows {
			desired = max(desired, min(displayWidth(row[i]), maxTableColumnWidth(column)))
		}
		extra := min(remaining, max(0, desired-widths[i]))
		widths[i] += extra
		remaining -= extra
	}

	selectedRows := make([][]string, len(rows))
	for i, row := range rows {
		selectedRows[i] = make([]string, len(selected))
		for j := range selected {
			selectedRows[i][j] = truncateTableCell(row[j], widths[j])
		}
	}
	for i, column := range selected {
		selected[i] = truncateTableCell(column, widths[i])
	}
	return selected, selectedRows, widths, len(columns) - len(selected)
}

func cellBudgetFor(widths []int) int {
	total := 0
	for _, width := range widths {
		total += width
	}
	return total
}

func minTableColumnWidth(column string) int {
	switch column {
	case "Zone":
		return 6
	case "ID", "Name":
		return 8
	case "Status":
		return 6
	case "Description":
		return 10
	case "Tags":
		return 6
	default:
		return 5
	}
}

func maxTableColumnWidth(column string) int {
	switch column {
	case "Zone":
		return 20
	case "ID":
		return 24
	case "Name", "Description":
		return 32
	default:
		return 24
	}
}

func writeTableBorder(writer io.Writer, widths []int) error {
	var line strings.Builder
	line.WriteByte('+')
	for _, width := range widths {
		line.WriteString(strings.Repeat("-", width+2))
		line.WriteByte('+')
	}
	line.WriteByte('\n')
	if _, err := io.WriteString(writer, line.String()); err != nil {
		return fmt.Errorf("write table border: %w", err)
	}
	return nil
}

func writeEmptyTable(writer io.Writer, terminalWidth int) error {
	width := min(displayWidth("No results"), max(1, terminalWidth-4))
	cell := truncateTableCell("No results", width)
	if err := writeTableBorder(writer, []int{width}); err != nil {
		return err
	}
	if err := writeTableRow(writer, []string{cell}, []int{width}); err != nil {
		return err
	}
	return writeTableBorder(writer, []int{width})
}

func writeTableRow(writer io.Writer, values []string, widths []int) error {
	var line strings.Builder
	line.WriteByte('|')
	for i, value := range values {
		line.WriteByte(' ')
		line.WriteString(value)
		line.WriteString(strings.Repeat(" ", max(0, widths[i]-displayWidth(value))))
		line.WriteString(" |")
	}
	line.WriteByte('\n')
	if _, err := io.WriteString(writer, line.String()); err != nil {
		return fmt.Errorf("write table row: %w", err)
	}
	return nil
}

func truncateTableCell(value string, maxWidth int) string {
	if displayWidth(value) <= maxWidth {
		return value
	}
	if maxWidth <= 3 {
		var truncated strings.Builder
		used := 0
		for _, r := range value {
			runeWidth := runeDisplayWidth(r)
			if used+runeWidth > maxWidth {
				break
			}
			truncated.WriteRune(r)
			used += runeWidth
		}
		return truncated.String()
	}

	var truncated strings.Builder
	used := 0
	for _, r := range value {
		runeWidth := runeDisplayWidth(r)
		if used+runeWidth > maxWidth-3 {
			break
		}
		truncated.WriteRune(r)
		used += runeWidth
	}
	return truncated.String() + "..."
}

func displayWidth(value string) int {
	total := 0
	for _, r := range value {
		total += runeDisplayWidth(r)
	}
	return total
}

func runeDisplayWidth(r rune) int {
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.IsControl(r) {
		return 0
	}
	switch width.LookupRune(r).Kind() {
	case width.EastAsianWide, width.EastAsianFullwidth:
		return 2
	default:
		return 1
	}
}

func orderedTableColumns(fields map[string]struct{}, includeZone bool) []string {
	priority := []string{
		"Zone", "ID", "Name", "Status", "Description", "Tags", "Availability",
		"ServiceClass", "Scope", "ServerCount", "NetworkMaskLen", "DefaultRoute",
		"CreatedAt", "ModifiedAt", "Provider", "Settings", "SettingsHash", "IconID",
		"Subnets", "BridgeID", "HybridConnectionID",
	}
	columns := make([]string, 0, len(fields)+1)
	if includeZone {
		columns = append(columns, "Zone")
	}
	for _, field := range priority {
		if field == "Zone" && includeZone {
			delete(fields, field)
			continue
		}
		if _, ok := fields[field]; ok {
			columns = append(columns, field)
			delete(fields, field)
		}
	}
	remaining := make([]string, 0, len(fields))
	for field := range fields {
		remaining = append(remaining, field)
	}
	sort.Strings(remaining)
	return append(columns, remaining...)
}

func tableValue(value any) string {
	switch value := value.(type) {
	case nil:
		return ""
	case string:
		return strings.NewReplacer("\t", " ", "\n", " ", "\r", " ").Replace(value)
	case []any:
		items := make([]string, len(value))
		for i, item := range value {
			items[i] = tableValue(item)
		}
		return strings.Join(items, ", ")
	case map[string]any:
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(data)
	default:
		return fmt.Sprint(value)
	}
}

func tableCellValue(field string, value any) string {
	if strings.HasSuffix(field, "At") {
		if timestamp, ok := value.(string); ok {
			parsed, err := time.Parse(time.RFC3339Nano, timestamp)
			if err == nil && parsed.IsZero() {
				return ""
			}
		}
	}
	return tableValue(value)
}
