package recon

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// NucleiHit is one CVE/template finding against a URL.
type NucleiHit struct {
	URL         string
	TemplateID  string
	TemplateURL string
	CVEs        []string
	Severity    string
	MatcherName string
	Evidence    map[string]any
}

// runNuclei feeds httpx URLs into nuclei via stdin and streams hits.
// Templates dir comes from the curated SilkStrand bundle.
func runNuclei(ctx context.Context, urls []string, onHit func(NucleiHit)) error {
	if len(urls) == 0 {
		return nil
	}
	bin, err := EnsureTool("nuclei")
	if err != nil {
		return fmt.Errorf("nuclei install: %w", err)
	}
	templatesDir, err := EnsureTemplates()
	if err != nil {
		return fmt.Errorf("nuclei templates: %w", err)
	}
	args := []string{
		"-jsonl",
		"-silent",
		"-no-color",
		"-disable-update-check",
		"-templates-directory", templatesDir,
		"-severity", "medium,high,critical",
		"-etags", "intrusive,fuzz,dast",
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("nuclei stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("nuclei stdout: %w", err)
	}
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("nuclei start: %w", err)
	}
	go drainStderr("nuclei", stderr)

	go func() {
		defer stdin.Close()
		for _, u := range urls {
			_, _ = stdin.Write([]byte(u + "\n"))
		}
	}()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 256*1024), 4*1024*1024)
	for scanner.Scan() {
		var line struct {
			TemplateID  string `json:"template-id"`
			TemplateURL string `json:"template-url"`
			Info        struct {
				Severity string   `json:"severity"`
				Classification struct {
					CVEID []string `json:"cve-id"`
				} `json:"classification"`
			} `json:"info"`
			MatchedAt   string         `json:"matched-at"`
			MatcherName string         `json:"matcher-name"`
			Extracted   map[string]any `json:"extracted-results"`
			Response    string         `json:"response"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		evidence := map[string]any{}
		if line.MatcherName != "" {
			evidence["matcher_name"] = line.MatcherName
		}
		if line.Extracted != nil {
			evidence["extracted"] = line.Extracted
		}
		if line.Response != "" {
			evidence["response"] = line.Response
		}
		// Redact secrets and truncate the body in place before the
		// hit is handed to the streamer.
		if raw, _ := json.Marshal(evidence); raw != nil {
			redacted := JSON(raw)
			_ = json.Unmarshal(redacted, &evidence)
		}
		onHit(NucleiHit{
			URL:         line.MatchedAt,
			TemplateID:  line.TemplateID,
			TemplateURL: line.TemplateURL,
			CVEs:        line.Info.Classification.CVEID,
			Severity:    line.Info.Severity,
			MatcherName: line.MatcherName,
			Evidence:    evidence,
		})
	}
	if err := scanner.Err(); err != nil {
		_ = cmd.Wait()
		return fmt.Errorf("nuclei stdout scan: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		// Nuclei exits non-zero when it finds nothing — treat that as success.
		if msg := err.Error(); strings.Contains(msg, "exit status 1") {
			return nil
		}
		return fmt.Errorf("nuclei exit: %w", err)
	}
	return nil
}
