// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// History is local diagnostic evidence, not an authenticated or tamper-proof ledger.
type historyEvent struct {
	Stdout         string            `json:"stdout,omitempty"`
	Stderr         string            `json:"stderr,omitempty"`
	Schema         int               `json:"schema"`
	ID             string            `json:"id"`
	Kind           string            `json:"kind"`
	Time           time.Time         `json:"time"`
	User           string            `json:"user,omitempty"`
	Host           string            `json:"host,omitempty"`
	Version        string            `json:"zap_version,omitempty"`
	CWD            string            `json:"cwd,omitempty"`
	Args           []string          `json:"args,omitempty"`
	ExitCode       *int              `json:"exit_code,omitempty"`
	Error          string            `json:"error,omitempty"`
	Message        string            `json:"message,omitempty"`
	Reference      string            `json:"reference,omitempty"`
	State          map[string]string `json:"state,omitempty"`
	DurationMS     int64             `json:"duration_ms,omitempty"`
	BuildSucceeded bool              `json:"build_succeeded,omitempty"`
}
type operationHistory struct {
	id, root, dir  string
	events         *os.File
	start          time.Time
	buildSucceeded bool
	mu             sync.Mutex
	failure        error
}

// The CLI dispatches one operation at a time. Independent processes use distinct files.
var currentHistory *operationHistory

func historyDigest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

var historyIDPattern = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}\.[0-9]{9}Z-[0-9a-f]{16}$`)
var historySecretPattern = regexp.MustCompile(`(?i)((?:password|passwd|token|secret|api[_-]?key|authorization)[ \t]*[:=][ \t]*)([^\s&]+)`)

func historyRedact(s string) string {
	return historySecretPattern.ReplaceAllString(redactSensitiveText(s), "${1}<redacted>")
}
func historyArgs(args []string) []string {
	out := make([]string, len(args))
	secretNext := false
	for i, arg := range args {
		if secretNext {
			out[i] = "<redacted>"
			secretNext = false
			continue
		}
		out[i] = historyRedact(arg)
		key := strings.ToLower(strings.TrimLeft(arg, "-"))
		switch key {
		case "password", "passwd", "token", "secret", "api-key", "api_key", "authorization":
			secretNext = true
		}
	}
	return out
}
func historyRoot(cwd string) string {
	fallback := cwd
	foundGit := false
	for dir := cwd; ; dir = filepath.Dir(dir) {
		if st, err := os.Stat(filepath.Join(dir, "zap.yml")); err == nil && !st.IsDir() {
			return dir
		}
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil && !foundGit {
			fallback = dir
			foundGit = true
		}
		if filepath.Dir(dir) == dir {
			return fallback
		}
	}
}
func ensureHistoryDirectory(path string) error {
	if err := os.Mkdir(path, 0700); err != nil && !os.IsExist(err) {
		return err
	}
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("history directory must be a real directory: %s", path)
	}
	return nil
}
func historyDirectory(root string, create bool) (string, error) {
	dir := root
	for _, name := range []string{".zap", "history", "operations"} {
		dir = filepath.Join(dir, name)
		if create {
			if err := ensureHistoryDirectory(dir); err != nil {
				return "", err
			}
		} else {
			st, err := os.Lstat(dir)
			if err != nil {
				return "", err
			}
			if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("unsafe history directory: %s", dir)
			}
		}
	}
	if create {
		// Keep recordings out of Git without modifying the user's project .gitignore.
		ignore := filepath.Join(root, ".zap", ".gitignore")
		if st, err := os.Lstat(ignore); err == nil && (!st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0) {
			return "", fmt.Errorf("unsafe history ignore file")
		}
		if err := ensureHistoryIgnore(ignore); err != nil {
			return "", err
		}
	}
	return dir, nil
}
func newHistory(root string, args []string) (*operationHistory, error) {
	dir, err := historyDirectory(root, true)
	if err != nil {
		return nil, fmt.Errorf("cannot start history recording: %w", err)
	}
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return nil, err
	}
	start := time.Now().UTC()
	id := start.Format("20060102T150405.000000000Z") + "-" + hex.EncodeToString(random[:])
	dir = filepath.Join(dir, id)
	if err := os.Mkdir(dir, 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	h := &operationHistory{id: id, root: root, dir: dir, events: f, start: start}
	who := "unknown"
	if u, err := user.Current(); err == nil {
		who = u.Username
	}
	host, _ := os.Hostname()
	cwd, _ := os.Getwd()
	state, err := h.snapshot("before")
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("cannot record initial state; command not started: %w", err)
	}
	err = h.event(historyEvent{Kind: "started", Time: start, User: who, Host: host, Version: Version, CWD: cwd, Args: historyArgs(args), State: state})
	if err != nil {
		f.Close()
		return nil, err
	}
	return h, nil
}
func (h *operationHistory) event(e historyEvent) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	e.Schema, e.ID = 1, h.id
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	if err := json.NewEncoder(h.events).Encode(e); err != nil {
		return err
	}
	if err := h.events.Sync(); err != nil {
		return err
	}
	return nil
}
func (h *operationHistory) recordFailure(err error) {
	if err == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.failure == nil {
		h.failure = err
		// Use the terminal saved by the output recorder, not its pipe.
		fmt.Fprintf(historyWarningOutput, "Warning: history recording failed; this operation's record is incomplete: %v\n", err)
	}
}
func (h *operationHistory) snapshot(name string) (map[string]string, error) {
	dir := filepath.Join(h.dir, name)
	if err := os.Mkdir(dir, 0700); err != nil {
		return nil, err
	}
	state := map[string]string{}
	for _, file := range []string{"zap.yml", "zap.lock"} {
		data, err := os.ReadFile(filepath.Join(h.root, file))
		if os.IsNotExist(err) {
			state[file] = "absent"
			continue
		}
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		state[file] = hex.EncodeToString(sum[:])
		if err := os.WriteFile(filepath.Join(dir, file), data, 0600); err != nil {
			return nil, err
		}
	}
	return state, nil
}
func (h *operationHistory) finish(args []string, commandErr error) error {
	state, err := h.snapshot("after")
	h.recordFailure(err)
	if err == nil {
		h.recordDependencyChanges()
	}
	code := ExitCode(commandErr)
	message := ""
	if commandErr != nil {
		message = historyRedact(commandErr.Error())
	}
	build := h.buildSucceeded && code == 0
	for _, arg := range args {
		if arg == "--configure-only" || strings.HasPrefix(arg, "--configure-only=") {
			build = false
		}
	}
	h.mu.Lock()
	incomplete := h.failure
	h.mu.Unlock()
	kind := "finished"
	if incomplete != nil {
		kind = "incomplete"
	}
	err = h.event(historyEvent{Kind: kind, ExitCode: &code, Error: message, State: state, DurationMS: time.Since(h.start).Milliseconds(), BuildSucceeded: build && incomplete == nil})
	h.recordFailure(err)
	h.recordFailure(h.events.Close())
	h.mu.Lock()
	incomplete = h.failure
	h.mu.Unlock()
	if incomplete != nil {
		return fmt.Errorf("history incomplete: %w", incomplete)
	}
	return nil
}
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() > 0 {
		return exit.ExitCode()
	}
	return 1
}

func Run(args []string) (result error) {
	if len(args) > 0 {
		switch strings.ToLower(args[0]) {
		case "help", "-h", "--help", "--version", "version-tool":
			return runCommand(args)
		case "log":
			return runHistoryLog(args[1:])
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	h, err := newHistory(historyRoot(cwd), args)
	if err != nil {
		return err
	}
	stop, err := h.captureOutput()
	if err != nil {
		_ = h.finish(args, err)
		return err
	}
	currentHistory = h
	h.recordExternalChanges()
	defer func() {
		currentHistory = nil
		stop()
		historyErr := h.finish(args, result)
		result = errors.Join(result, historyErr)
	}()
	if len(args) > 0 && (strings.EqualFold(args[0], "e") || strings.EqualFold(args[0], "exec")) {
		if _, err := os.Stat(filepath.Join(h.root, "zap.yml")); err == nil {
			p := OpenProject(h.root)
			p.warnIntegrity("before exec")
			defer p.warnIntegrity("after exec")
		}
		command := args[1:]
		if len(command) > 0 && command[0] == "--" {
			command = command[1:]
		}
		if len(command) == 0 {
			return fmt.Errorf("usage: zap e <executable> [arguments...]")
		}
		// A user command intentionally inherits its environment. It is not an
		// internal dependency Git operation and must not use sanitizedGitEnv.
		cmd := exec.Command(command[0], command[1:]...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s: %w", command[0], err)
		}
		return nil
	}
	return runCommand(args)
}

// Tool events include commands used internally, even when their output is captured
// for parsing rather than printed to the terminal.
func beginHistoryTool(dir, program string, args []string) func(error, string, string) {
	h := currentHistory
	if h == nil {
		return func(error, string, string) {}
	}
	command := append([]string{program}, args...)
	h.recordFailure(h.event(historyEvent{Kind: "tool-started", CWD: dir, Args: historyArgs(command)}))
	return func(err error, stdout, stderr string) {
		code := ExitCode(err)
		message := ""
		if err != nil {
			message = historyRedact(err.Error())
		}
		h.recordFailure(h.event(historyEvent{Kind: "tool-finished", Args: historyArgs(command), ExitCode: &code, Error: message, Stdout: historyRedact(stdout), Stderr: historyRedact(stderr)}))
	}
}

type recordedHistory struct {
	dir        string
	events     []historyEvent
	incomplete bool
}

func readHistories(root string) ([]recordedHistory, error) {
	base, err := historyDirectory(root, false)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}
	var records []recordedHistory
	for _, entry := range entries {
		if !historyIDPattern.MatchString(entry.Name()) {
			continue
		}
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("unsafe history entry %s", entry.Name())
		}
		dir := filepath.Join(base, entry.Name())
		path := filepath.Join(dir, "events.jsonl")
		st, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if !st.Mode().IsRegular() {
			return nil, fmt.Errorf("unsafe history event file")
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		r := recordedHistory{dir: dir}
		dec := json.NewDecoder(f)
		for {
			var e historyEvent
			err := dec.Decode(&e)
			if err == io.EOF {
				break
			}
			if err != nil {
				r.incomplete = true
				break
			}
			if e.ID != entry.Name() || e.Schema != 1 {
				r.incomplete = true
				break
			}
			r.events = append(r.events, e)
		}
		f.Close()
		records = append(records, r)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].dir < records[j].dir })
	return records, nil
}
func findHistory(records []recordedHistory, id string) (recordedHistory, error) {
	if !historyIDPattern.MatchString(id) {
		return recordedHistory{}, fmt.Errorf("use the complete operation ID shown by zap log")
	}
	for _, r := range records {
		if filepath.Base(r.dir) == id {
			return r, nil
		}
	}
	return recordedHistory{}, fmt.Errorf("history operation %s not found", id)
}
func runHistoryLog(args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root := historyRoot(cwd)
	if len(args) > 0 && args[0] == "-m" {
		fs := flag.NewFlagSet("log", flag.ContinueOnError)
		message := fs.String("m", "", "user note")
		if err := fs.Parse(args); err != nil {
			return err
		}
		if strings.TrimSpace(*message) == "" || fs.NArg() != 0 {
			return fmt.Errorf("usage: zap log -m \"note\"")
		}
		return writeHistoryNote(root, args, "note", "", *message)
	}
	records, err := readHistories(root)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		for _, r := range records {
			if len(r.events) == 0 {
				fmt.Printf("%s INCOMPLETE\n", filepath.Base(r.dir))
				continue
			}
			first := r.events[0]
			status := "RUNNING/INTERRUPTED"
			comment := ""
			var details []string
			for _, e := range r.events {
				switch e.Kind {
				case "finished":
					if e.ExitCode != nil && *e.ExitCode == 0 {
						status = "OK"
					} else {
						status = "FAILED"
					}
				case "incomplete":
					status = "INCOMPLETE"
				case "dependency-changed":
					details = append(details, fmt.Sprintf("dependency %s changed (use zap log show %s for before/after)", e.Reference, first.ID))
				case "external-state-change":
					details = append(details, e.Message)
				case "note", "known-good":
					comment = e.Message
				}
			}
			if r.incomplete {
				status = "INCOMPLETE"
			}
			fmt.Printf("%s  %s  %s  %s\n", first.ID, status, first.User, strings.Join(first.Args, " "))
			for _, detail := range details {
				fmt.Printf("  %s\n", detail)
			}
			if comment != "" {
				fmt.Printf("  %s\n", comment)
			}
		}
		if len(records) == 0 {
			fmt.Println("No recorded history.")
		}
		return nil
	}
	switch args[0] {
	case "show":
		if len(args) < 2 || len(args) > 3 {
			return fmt.Errorf("usage: zap log show <id> [--output]")
		}
		if len(args) == 3 && args[2] != "--output" {
			return fmt.Errorf("expected --output")
		}
		r, err := findHistory(records, args[1])
		if err != nil {
			return err
		}
		for _, e := range r.events {
			data, _ := json.MarshalIndent(e, "", "  ")
			fmt.Println(string(data))
		}
		if r.incomplete {
			fmt.Println("WARNING: malformed or incomplete history.")
		}
		if len(args) == 3 {
			for _, name := range []string{"stdout.log", "stderr.log"} {
				path := filepath.Join(r.dir, name)
				st, err := os.Lstat(path)
				if os.IsNotExist(err) {
					continue
				}
				if err != nil {
					return err
				}
				if !st.Mode().IsRegular() {
					return fmt.Errorf("unsafe output file")
				}
				f, err := os.Open(path)
				if err != nil {
					return err
				}
				fmt.Printf("\n--- %s ---\n", name)
				_, err = io.Copy(os.Stdout, f)
				f.Close()
				if err != nil {
					return err
				}
			}
		}
		return nil
	case "diff":
		if len(args) != 2 && len(args) != 3 {
			return fmt.Errorf("usage: zap log diff <id> [other-id]")
		}
		a, err := findHistory(records, args[1])
		if err != nil {
			return err
		}
		left, right := filepath.Join(a.dir, "before"), filepath.Join(a.dir, "after")
		if len(args) == 3 {
			b, err := findHistory(records, args[2])
			if err != nil {
				return err
			}
			left = filepath.Join(a.dir, "after")
			right = filepath.Join(b.dir, "after")
		}
		return printHistoryDiff(left, right)
	case "mark-good":
		if len(args) < 2 {
			return fmt.Errorf("usage: zap log mark-good <id> -m \"validation note\"")
		}
		r, err := findHistory(records, args[1])
		if err != nil {
			return err
		}
		good := false
		for _, e := range r.events {
			if e.Kind == "finished" && e.BuildSucceeded {
				good = true
			}
		}
		if !good || r.incomplete {
			return fmt.Errorf("only a completely recorded successful make/build can be marked known good")
		}
		fs := flag.NewFlagSet("mark-good", flag.ContinueOnError)
		message := fs.String("m", "", "validation note")
		if err := fs.Parse(args[2:]); err != nil {
			return err
		}
		if strings.TrimSpace(*message) == "" || fs.NArg() != 0 {
			return fmt.Errorf("a validation note (-m) is required")
		}
		return writeHistoryNote(root, args, "known-good", args[1], *message)
	default:
		return fmt.Errorf("usage: zap log [-m \"note\" | show <id> [--output] | diff <id> [other-id] | mark-good <id> -m \"note\"]")
	}
}
func writeHistoryNote(root string, args []string, kind, ref, message string) error {
	h, err := newHistory(root, append([]string{"log"}, args...))
	if err != nil {
		return err
	}
	err = h.event(historyEvent{Kind: kind, Reference: ref, Message: historyRedact(message)})
	finishErr := h.finish(nil, err)
	if err == nil && finishErr == nil {
		fmt.Printf("Recorded %s: %s\n", kind, h.id)
	}
	return errors.Join(err, finishErr)
}
func printHistoryDiff(left, right string) error {
	for _, name := range []string{"zap.yml", "zap.lock"} {
		read := func(dir string) ([]byte, error) {
			st, err := os.Lstat(dir)
			if err != nil {
				return nil, err
			}
			if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf("unsafe snapshot directory")
			}
			path := filepath.Join(dir, name)
			st, err = os.Lstat(path)
			if os.IsNotExist(err) {
				return []byte("(absent)\n"), nil
			}
			if err != nil {
				return nil, err
			}
			if !st.Mode().IsRegular() {
				return nil, fmt.Errorf("unsafe snapshot file")
			}
			return os.ReadFile(path)
		}
		a, err := read(left)
		if err != nil {
			return err
		}
		b, err := read(right)
		if err != nil {
			return err
		}
		if string(a) == string(b) {
			fmt.Printf("%s: unchanged\n", name)
			continue
		}
		fmt.Printf("--- %s before\n%s\n+++ %s after\n%s\n", name, historyRedact(string(a)), name, historyRedact(string(b)))
	}
	return nil
}
