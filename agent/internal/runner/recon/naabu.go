package recon

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// NaabuFinding is one host:port hit from a naabu JSON line.
type NaabuFinding struct {
	IP   string
	Port int
	Host string
}

// runNaabu invokes naabu against the given target and streams JSON
// findings to onFinding. Blocks until naabu exits or ctx is cancelled.
func runNaabu(ctx context.Context, target string, ratePPS int, onFinding func(NaabuFinding)) error {
	bin, err := EnsureTool("naabu")
	if err != nil {
		return fmt.Errorf("naabu install: %w", err)
	}
	args := []string{
		"-host", target,
		"-json",
		"-silent",
		"-rate", strconv.Itoa(ratePPS),
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("naabu stdout pipe: %w", err)
	}
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("naabu start: %w", err)
	}
	tail := drainStderr("naabu", stderr)

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var line struct {
			IP   string `json:"ip"`
			Host string `json:"host"`
			Port int    `json:"port"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue // tolerate malformed line, keep scanning
		}
		if line.IP == "" {
			continue
		}
		onFinding(NaabuFinding{IP: line.IP, Port: line.Port, Host: line.Host})
	}
	if err := scanner.Err(); err != nil {
		_ = cmd.Wait()
		return fmt.Errorf("naabu stdout scan: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("naabu exit: %w%s", err, tail.suffix())
	}
	return nil
}

// stderrTail keeps the last N stderr lines from a subprocess so they can
// be appended to the error returned when the process exits non-zero.
type stderrTail struct {
	mu    sync.Mutex
	lines []string
	max   int
}

func (t *stderrTail) add(line string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lines = append(t.lines, line)
	if len(t.lines) > t.max {
		t.lines = t.lines[len(t.lines)-t.max:]
	}
}

func (t *stderrTail) suffix() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.lines) == 0 {
		return ""
	}
	return ": " + strings.Join(t.lines, " | ")
}

func drainStderr(_ string, r io.ReadCloser) *stderrTail {
	tail := &stderrTail{max: 10}
	go func() {
		defer r.Close()
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			tail.add(scanner.Text())
		}
	}()
	return tail
}
