package recon

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
)

// HTTPXFinding is one URL probe with fingerprint data.
type HTTPXFinding struct {
	URL          string
	Host         string
	IP           string
	Port         int
	Title        string
	WebServer    string
	Technologies []string
}

// runHTTPX feeds (ip, port) pairs into httpx via stdin and streams
// fingerprint findings out. The agent serializes input in
// `host:port` lines, one per naabu finding.
func runHTTPX(ctx context.Context, inputs []string, onFinding func(HTTPXFinding)) error {
	if len(inputs) == 0 {
		return nil
	}
	bin, err := EnsureTool("httpx")
	if err != nil {
		return fmt.Errorf("httpx install: %w", err)
	}
	args := []string{
		"-json",
		"-silent",
		"-tech-detect",
		"-tls-grab",
		"-status-code",
		"-title",
		"-no-color",
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("httpx stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("httpx stdout: %w", err)
	}
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("httpx start: %w", err)
	}
	tail := drainStderr("httpx", stderr)

	go func() {
		defer stdin.Close()
		for _, in := range inputs {
			_, _ = stdin.Write([]byte(in + "\n"))
		}
	}()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var line struct {
			URL          string   `json:"url"`
			Host         string   `json:"host"`
			IP           string   `json:"ip"`
			Port         string   `json:"port"`
			Title        string   `json:"title"`
			Webserver    string   `json:"webserver"`
			Technologies []string `json:"tech"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		port, _ := strconv.Atoi(line.Port)
		onFinding(HTTPXFinding{
			URL:          line.URL,
			Host:         line.Host,
			IP:           line.IP,
			Port:         port,
			Title:        line.Title,
			WebServer:    line.Webserver,
			Technologies: line.Technologies,
		})
	}
	if err := scanner.Err(); err != nil {
		_ = cmd.Wait()
		return fmt.Errorf("httpx stdout scan: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("httpx exit: %w%s", err, tail.suffix())
	}
	return nil
}

