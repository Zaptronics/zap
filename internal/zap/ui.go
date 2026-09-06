// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
)

const (
	ansiReset   = "\x1b[0m"
	ansiBold    = "\x1b[1m"
	ansiDim     = "\x1b[2m"
	ansiRed     = "\x1b[31m"
	ansiGreen   = "\x1b[32m"
	ansiYellow  = "\x1b[33m"
	ansiBlue    = "\x1b[34m"
	ansiMagenta = "\x1b[35m"
	ansiCyan    = "\x1b[36m"
	ansiWhite   = "\x1b[37m"
)

type uiRow struct {
	Label string
	Value string
}

func ansiEnabled(f *os.File) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("ZAP_COLOR"))) {
	case "always", "1", "true", "yes", "on":
		return true
	case "never", "0", "false", "no", "off":
		return false
	}
	if strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return false
	}
	return enableANSI(f)
}

func paint(enabled bool, code, s string) string {
	if !enabled || s == "" {
		return s
	}
	return code + s + ansiReset
}

// displayWidth returns the number of terminal columns occupied by s. Terminals
// commonly render emoji and East Asian wide characters as two columns even
// though they are a single Unicode code point. Combining marks occupy none.
func displayWidth(s string) int {
	width := 0
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Mn, r), unicode.Is(unicode.Me, r), r == '\u200d', r == '\ufe0f':
			// Combining marks, emoji joiners, and variation selectors add no width.
		case isWideRune(r):
			width += 2
		default:
			width++
		}
	}
	return width
}

func isWideRune(r rune) bool {
	// Covers the common CJK/full-width ranges plus emoji/symbol ranges used by
	// Zap's terminal UI. In particular U+26A1 HIGH VOLTAGE SIGN is rendered
	// two columns wide by Windows Terminal and most modern terminal emulators.
	return r == '⚡' ||
		(r >= 0x1100 && r <= 0x115f) ||
		(r >= 0x2329 && r <= 0x232a) ||
		(r >= 0x2e80 && r <= 0xa4cf) ||
		(r >= 0xac00 && r <= 0xd7a3) ||
		(r >= 0xf900 && r <= 0xfaff) ||
		(r >= 0xfe10 && r <= 0xfe19) ||
		(r >= 0xfe30 && r <= 0xfe6f) ||
		(r >= 0xff00 && r <= 0xff60) ||
		(r >= 0xffe0 && r <= 0xffe6) ||
		(r >= 0x1f300 && r <= 0x1faff) ||
		(r >= 0x20000 && r <= 0x3fffd)
}

func uiBanner(title, detail string) {
	if !ansiEnabled(os.Stdout) {
		if detail != "" {
			fmt.Printf("==> %s: %s\n", title, detail)
		} else {
			fmt.Printf("==> %s\n", title)
		}
		return
	}

	title = strings.ToUpper(strings.TrimSpace(title))
	brand := " ⚡ ZAP"
	suffix := " · " + title + " "
	captionWidth := displayWidth(brand) + displayWidth(suffix)
	inner := captionWidth + 2
	if detailWidth := displayWidth(detail) + 4; detailWidth > inner {
		inner = detailWidth
	}
	if inner < 56 {
		inner = 56
	}
	if inner > 94 {
		inner = 94
	}

	fill := inner - 1 - captionWidth
	if fill < 1 {
		fill = 1
	}
	fmt.Printf("%s%s%s%s%s\n",
		paint(true, ansiCyan+ansiDim, "╭─"),
		paint(true, ansiBold+ansiYellow, brand),
		paint(true, ansiBold+ansiCyan, suffix),
		paint(true, ansiCyan+ansiDim, strings.Repeat("─", fill)),
		paint(true, ansiCyan+ansiDim, "╮"))
	if detail != "" {
		line := "  " + detail
		if displayWidth(line) > inner {
			line = truncateToDisplayWidth(line, inner-1) + "…"
		}
		pad := inner - displayWidth(line)
		fmt.Printf("%s%s%s%s\n", paint(true, ansiCyan+ansiDim, "│"), paint(true, ansiWhite, line), strings.Repeat(" ", pad), paint(true, ansiCyan+ansiDim, "│"))
	}
	fmt.Printf("%s%s%s\n", paint(true, ansiCyan+ansiDim, "╰"), paint(true, ansiCyan+ansiDim, strings.Repeat("─", inner)), paint(true, ansiCyan+ansiDim, "╯"))
}

func truncateToDisplayWidth(s string, max int) string {
	if max <= 0 {
		return ""
	}
	width := 0
	var b strings.Builder
	for _, r := range s {
		rw := displayWidth(string(r))
		if width+rw > max {
			break
		}
		b.WriteRune(r)
		width += rw
	}
	return b.String()
}

func uiSection(title string) {
	if ansiEnabled(os.Stdout) {
		fmt.Printf("\n  %s %s\n", paint(true, ansiCyan, "◆"), paint(true, ansiBold, title))
		return
	}
	fmt.Printf("\n-- %s --\n", title)
}

func uiStep(label, detail string) {
	enabled := ansiEnabled(os.Stdout)
	if enabled {
		fmt.Printf("  %s %-12s %s\n", paint(true, ansiCyan, "◇"), paint(true, ansiBold, label), paint(true, ansiWhite, detail))
		return
	}
	if detail == "" {
		fmt.Printf("  > %s\n", label)
	} else {
		fmt.Printf("  > %-12s %s\n", label, detail)
	}
}

func uiSuccess(message string) {
	if ansiEnabled(os.Stdout) {
		fmt.Printf("  %s %s\n", paint(true, ansiGreen+ansiBold, "✓"), message)
		return
	}
	fmt.Printf("  OK %s\n", message)
}

func uiWarning(message string) {
	if ansiEnabled(os.Stdout) {
		fmt.Printf("  %s %s\n", paint(true, ansiYellow+ansiBold, "!"), paint(true, ansiYellow, message))
		return
	}
	fmt.Printf("  ! %s\n", message)
}

func uiDetail(label, value string) {
	if ansiEnabled(os.Stdout) {
		fmt.Printf("      %s  %s\n", paint(true, ansiDim, fmt.Sprintf("%-11s", label)), value)
		return
	}
	fmt.Printf("      %-11s %s\n", label, value)
}

func uiHint(message string) {
	if ansiEnabled(os.Stdout) {
		fmt.Printf("  %s %s\n", paint(true, ansiMagenta, "↳"), paint(true, ansiDim, message))
		return
	}
	fmt.Printf("  -> %s\n", message)
}

func uiResult(title string, rows ...uiRow) {
	enabled := ansiEnabled(os.Stdout)
	if !enabled {
		fmt.Printf("\n%s\n", title)
		for _, row := range rows {
			fmt.Printf("  %-16s %s\n", row.Label+":", row.Value)
		}
		return
	}

	brand := " ⚡ ZAP"
	suffix := " · " + title + " "
	captionWidth := displayWidth(brand) + displayWidth(suffix)
	inner := captionWidth + 3
	for _, row := range rows {
		w := 16 + displayWidth(row.Value)
		if w > inner {
			inner = w
		}
	}
	if inner < 56 {
		inner = 56
	}
	if inner > 96 {
		inner = 96
	}
	fill := inner - 1 - captionWidth
	if fill < 1 {
		fill = 1
	}
	fmt.Printf("\n%s%s%s%s%s\n",
		paint(true, ansiGreen+ansiDim, "╭─"),
		paint(true, ansiYellow+ansiBold, brand),
		paint(true, ansiGreen+ansiBold, suffix),
		paint(true, ansiGreen+ansiDim, strings.Repeat("─", fill)),
		paint(true, ansiGreen+ansiDim, "╮"))
	for _, row := range rows {
		value := row.Value
		maxValue := inner - 16
		if displayWidth(value) > maxValue {
			value = truncateToDisplayWidth(value, maxValue-1) + "…"
		}
		content := fmt.Sprintf(" %-14s %s", row.Label, value)
		pad := inner - displayWidth(content)
		fmt.Printf("%s%s%s%s%s\n", paint(true, ansiGreen+ansiDim, "│"), paint(true, ansiDim, fmt.Sprintf(" %-14s", row.Label)), " ", value, strings.Repeat(" ", pad)+paint(true, ansiGreen+ansiDim, "│"))
	}
	fmt.Printf("%s%s%s\n", paint(true, ansiGreen+ansiDim, "╰"), paint(true, ansiGreen+ansiDim, strings.Repeat("─", inner)), paint(true, ansiGreen+ansiDim, "╯"))
}

func uiHelpHeader() {
	enabled := ansiEnabled(os.Stdout)
	if !enabled {
		fmt.Printf("⚡ Zap %s - embedded project + dependency tooling\n", Version)
		return
	}

	const inner = 66
	brand := " ⚡ ZAP"
	suffix := " · HELP "
	captionWidth := displayWidth(brand) + displayWidth(suffix)
	fill := inner - 1 - captionWidth
	if fill < 1 {
		fill = 1
	}
	fmt.Printf("%s%s%s%s%s\n",
		paint(true, ansiCyan+ansiDim, "╭─"),
		paint(true, ansiBold+ansiYellow, brand),
		paint(true, ansiBold+ansiCyan, suffix),
		paint(true, ansiCyan+ansiDim, strings.Repeat("─", fill)),
		paint(true, ansiCyan+ansiDim, "╮"))

	detail := fmt.Sprintf("  Embedded project + dependency tooling · %s", Version)
	pad := inner - displayWidth(detail)
	if pad < 0 {
		pad = 0
	}
	fmt.Printf("%s%s%s%s\n", paint(true, ansiCyan+ansiDim, "│"), paint(true, ansiWhite, detail), strings.Repeat(" ", pad), paint(true, ansiCyan+ansiDim, "│"))
	fmt.Printf("%s%s%s\n", paint(true, ansiCyan+ansiDim, "╰"), paint(true, ansiCyan+ansiDim, strings.Repeat("─", inner)), paint(true, ansiCyan+ansiDim, "╯"))
}

func uiHelpUsage(usage string) {
	enabled := ansiEnabled(os.Stdout)
	if enabled {
		fmt.Printf("\n  %s  %s\n", paint(true, ansiDim, "Usage"), paint(true, ansiBold+ansiWhite, usage))
		return
	}
	fmt.Printf("\nUsage: %s\n", usage)
}

func uiHelpSection(title string) {
	enabled := ansiEnabled(os.Stdout)
	if enabled {
		fmt.Printf("\n  %s %s\n", paint(true, ansiCyan, "◆"), paint(true, ansiBold+ansiCyan, strings.ToUpper(title)))
		return
	}
	fmt.Printf("\n%s:\n", title)
}

func uiHelpCommand(command, args, description string) {
	enabled := ansiEnabled(os.Stdout)
	invocation := command
	if args != "" {
		invocation += " " + args
	}
	if enabled {
		fmt.Printf("    %s %-31s %s\n", paint(true, ansiYellow, "›"), paint(true, ansiBold+ansiWhite, invocation), paint(true, ansiDim, description))
		return
	}
	fmt.Printf("  %-33s %s\n", invocation, description)
}

func uiHelpNote(message string) {
	enabled := ansiEnabled(os.Stdout)
	if enabled {
		fmt.Printf("\n  %s %s\n", paint(true, ansiYellow+ansiBold, "⚡"), paint(true, ansiDim, message))
		return
	}
	fmt.Printf("\n  %s\n", message)
}

// PrintError prints a Zap error using the same terminal presentation as normal output.
func PrintError(w io.Writer, err error) {
	if err == nil {
		return
	}
	f, ok := w.(*os.File)
	enabled := ok && ansiEnabled(f)
	if enabled {
		fmt.Fprintf(w, "%s %s\n", paint(true, ansiRed+ansiBold, "✗ zap"), paint(true, ansiRed, err.Error()))
		return
	}
	fmt.Fprintf(w, "zap: %v\n", err)
}
