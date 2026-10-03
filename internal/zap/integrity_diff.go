// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

type auditDiffLine struct {
	kind byte
	text string
}

func auditLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	lines := strings.SplitAfter(string(data), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func auditLineDiff(a, b []string) []auditDiffLine {
	var out []auditDiffLine
	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		out = append(out, auditDiffLine{' ', a[prefix]})
		prefix++
	}
	a, b = a[prefix:], b[prefix:]
	suffix := 0
	for suffix < len(a) && suffix < len(b) && a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		suffix++
	}
	tail := a[len(a)-suffix:]
	a, b = a[:len(a)-suffix], b[:len(b)-suffix]
	// Bound memory/time for generated or completely rewritten files. The fallback
	// emits all changed lines, without silently omitting content.
	if len(a) > 0 && len(b) > 0 && len(a) <= 2000000/len(b) {
		width := len(b) + 1
		table := make([]int, (len(a)+1)*width)
		for i := len(a) - 1; i >= 0; i-- {
			for j := len(b) - 1; j >= 0; j-- {
				if a[i] == b[j] {
					table[i*width+j] = table[(i+1)*width+j+1] + 1
				} else {
					x, y := table[(i+1)*width+j], table[i*width+j+1]
					if y > x {
						x = y
					}
					table[i*width+j] = x
				}
			}
		}
		i, j := 0, 0
		for i < len(a) || j < len(b) {
			if i < len(a) && j < len(b) && a[i] == b[j] {
				out = append(out, auditDiffLine{' ', a[i]})
				i++
				j++
			} else if i < len(a) && (j == len(b) || table[(i+1)*width+j] >= table[i*width+j+1]) {
				out = append(out, auditDiffLine{'-', a[i]})
				i++
			} else {
				out = append(out, auditDiffLine{'+', b[j]})
				j++
			}
		}
	} else {
		for _, s := range a {
			out = append(out, auditDiffLine{'-', s})
		}
		for _, s := range b {
			out = append(out, auditDiffLine{'+', s})
		}
	}
	for _, s := range tail {
		out = append(out, auditDiffLine{' ', s})
	}
	return out
}

func writeAuditDiff(w io.Writer, ch integrityChange, before, after []byte, color bool) {
	fmt.Fprintf(w, "\n%s %s\n", paint(color, ansiBold+ansiYellow, ch.Kind), auditText(ch.Path))
	if ch.Before != nil && ch.After != nil && ch.Before.Executable != ch.After.Executable {
		fmt.Fprintf(w, "Executable: %t -> %t\n", ch.Before.Executable, ch.After.Executable)
	}
	if bytes.IndexByte(before, 0) >= 0 || bytes.IndexByte(after, 0) >= 0 || !utf8.Valid(before) || !utf8.Valid(after) {
		for _, s := range []struct {
			name string
			file *integrityFile
		}{{"before", ch.Before}, {"after", ch.After}} {
			if s.file == nil {
				fmt.Fprintf(w, "%s: absent\n", s.name)
			} else {
				fmt.Fprintf(w, "%s: binary SHA256=%s\n", s.name, s.file.SHA256)
			}
		}
		return
	}
	if bytes.Equal(before, after) {
		fmt.Fprintln(w, "File contents unchanged (metadata or presence changed).")
		return
	}
	oldName, newName := "a/"+auditText(ch.Path), "b/"+auditText(ch.Path)
	if ch.Before == nil {
		oldName = "/dev/null"
	}
	if ch.After == nil {
		newName = "/dev/null"
	}
	fmt.Fprintln(w, paint(color, ansiRed, "--- "+oldName))
	fmt.Fprintln(w, paint(color, ansiGreen, "+++ "+newName))
	ops := auditLineDiff(auditLines(before), auditLines(after))
	old, new := make([]int, len(ops)+1), make([]int, len(ops)+1)
	for i, op := range ops {
		old[i+1], new[i+1] = old[i], new[i]
		if op.kind != '+' {
			old[i+1]++
		}
		if op.kind != '-' {
			new[i+1]++
		}
	}
	for i := 0; i < len(ops); {
		if ops[i].kind == ' ' {
			i++
			continue
		}
		start := i - 3
		if start < 0 {
			start = 0
		}
		last := i
		for j := i + 1; j < len(ops) && j <= last+7; j++ {
			if ops[j].kind != ' ' {
				last = j
			}
		}
		end := last + 4
		if end > len(ops) {
			end = len(ops)
		}
		oc, nc := old[end]-old[start], new[end]-new[start]
		os, ns := old[start]+1, new[start]+1
		if oc == 0 {
			os = old[start]
		}
		if nc == 0 {
			ns = new[start]
		}
		fmt.Fprintln(w, paint(color, ansiCyan, fmt.Sprintf("@@ -%d,%d +%d,%d @@", os, oc, ns, nc)))
		for _, op := range ops[start:end] {
			code := ansiDim
			if op.kind == '-' {
				code = ansiRed
			}
			if op.kind == '+' {
				code = ansiGreen
			}
			raw := strings.TrimSuffix(op.text, "\n")
			cr := strings.HasSuffix(raw, "\r")
			if cr {
				raw = strings.TrimSuffix(raw, "\r")
			}
			text := auditText(strings.ReplaceAll(raw, "\\", "\\\\"))
			// Make trailing spaces visible without changing the comparison.
			trailing := len(text) - len(strings.TrimRight(text, " "))
			if trailing > 0 {
				text = text[:len(text)-trailing] + strings.Repeat("·", trailing)
			}
			if cr {
				text += "\\r"
			}
			fmt.Fprintln(w, paint(color, code, string(op.kind)+text))
			if !strings.HasSuffix(op.text, "\n") {
				fmt.Fprintln(w, paint(color, ansiYellow, "\\ No newline at end of file"))
			}
		}
		i = end
	}
}

func showAuditDiff(ch integrityChange, before, after []byte) {
	writeAuditDiff(os.Stdout, ch, before, after, ansiEnabled(os.Stdout))
}
