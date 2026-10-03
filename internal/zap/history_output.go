// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"sync"
)

// Terminal output is forwarded immediately. Recorded output is redacted a line
// at a time so secrets split across Write calls do not escape the redactor.
// No line/output length limit or automatic deletion is imposed.
var historyWarningOutput io.Writer = os.Stderr

// Strip hyperlink wrappers after buffering the complete line, including links
// split across pipe reads. Preserve the visible label and ordinary path text.
var historyHyperlink = regexp.MustCompile("\\x1b\\]8;[^\\x1b\\x07]*(?:\\x07|\\x1b\\\\)")

type historyOutput struct {
	h       *operationHistory
	file    *os.File
	pending bytes.Buffer
	failed  bool
}

func (w *historyOutput) Write(p []byte) (int, error) {
	n := len(p)
	if w.failed {
		return n, nil
	}
	for len(p) > 0 {
		i := bytes.IndexAny(p, "\r\n")
		if i < 0 {
			w.pending.Write(p)
			break
		}
		w.pending.Write(p[:i+1])
		w.flushLine()
		p = p[i+1:]
	}
	return n, nil
}
func (w *historyOutput) flushLine() {
	if w.failed {
		w.pending.Reset()
		return
	}
	_, err := io.WriteString(w.file, historyRedact(historyHyperlink.ReplaceAllString(w.pending.String(), "")))
	w.pending.Reset()
	if err != nil {
		w.failed = true
		w.h.recordFailure(err)
	}
}
func (w *historyOutput) close() {
	w.flushLine()
	w.h.recordFailure(w.file.Sync())
	w.h.recordFailure(w.file.Close())
}
func (h *operationHistory) captureOutput() (func(), error) {
	out, err := os.OpenFile(h.dir+string(os.PathSeparator)+"stdout.log", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	errout, err := os.OpenFile(h.dir+string(os.PathSeparator)+"stderr.log", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		out.Close()
		return nil, err
	}
	r1, w1, err := os.Pipe()
	if err != nil {
		out.Close()
		errout.Close()
		return nil, err
	}
	r2, w2, err := os.Pipe()
	if err != nil {
		r1.Close()
		w1.Close()
		out.Close()
		errout.Close()
		return nil, err
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	historyConsoleOut, historyConsoleErr = oldOut, oldErr
	oldWarning := historyWarningOutput
	historyWarningOutput = oldErr
	var wg sync.WaitGroup
	pump := func(r *os.File, terminal *os.File, file *os.File) {
		defer wg.Done()
		sink := &historyOutput{h: h, file: file}
		buf := make([]byte, 32*1024)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				// A log failure must not abort an already-running child or hide live output.
				if _, writeErr := terminal.Write(buf[:n]); writeErr != nil {
					h.recordFailure(fmt.Errorf("forward console output: %w", writeErr))
				}
				sink.Write(buf[:n])
			}
			if err != nil {
				if err != io.EOF {
					h.recordFailure(err)
				}
				break
			}
		}
		sink.close()
		r.Close()
	}
	wg.Add(2)
	go pump(r1, oldOut, out)
	go pump(r2, oldErr, errout)
	os.Stdout, os.Stderr = w1, w2
	return func() {
		os.Stdout, os.Stderr = oldOut, oldErr
		w1.Close()
		w2.Close()
		wg.Wait()
		historyConsoleOut, historyConsoleErr = nil, nil
		historyWarningOutput = oldWarning
	}, nil
}
