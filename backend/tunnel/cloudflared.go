package tunnel

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sync"
	"time"
)

var urlRegex = regexp.MustCompile(`https://[a-z0-9-]+\.trycloudflare\.com`)

// Status describes the cloudflared process state.
type Status string

const (
	StatusStopped  Status = "stopped"
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusError    Status = "error"
)

// Cloudflared manages a single cloudflared subprocess. It supports two
// connection modes that get picked at Start time based on which fields
// the caller populated:
//
//	trycloudflare (default)
//	  Started with `cloudflared tunnel --url http://127.0.0.1:<LocalPort>`.
//	  Public URL is a brand-new <random>.trycloudflare.com domain
//	  scraped out of cloudflared's own log line, surfaced via OnURL.
//	  The URL changes on every Start, so external clients need to
//	  re-pin it after each Prism restart.
//
//	named tunnel  (TunnelToken non-empty)
//	  Started with `cloudflared tunnel run --token <TunnelToken>`.
//	  cloudflared connects to a Cloudflare account-bound tunnel whose
//	  ingress + hostname are configured in the Cloudflare dashboard,
//	  giving you a stable hostname (e.g. prism.example.com) that
//	  survives restarts. Because the hostname isn't visible in the
//	  cloudflared logs, the caller MUST also populate PublicHostname
//	  so the UI / OnURL callback knows which URL to surface.
type Cloudflared struct {
	Binary    string
	LocalPort int
	LogWriter io.Writer // optional additional sink (e.g. EventsEmit bridge)
	OnURL     func(string)

	// TunnelToken, when non-empty, switches into named-tunnel mode.
	// The token is opaque to us: cloudflared decodes it and pulls
	// down the ingress configuration on its own. Empty falls back
	// to trycloudflare.
	TunnelToken string

	// PublicHostname is the user-visible URL (without scheme; e.g.
	// "prism.example.com") that named-tunnel mode should report to
	// OnURL once the connection is up. Ignored in trycloudflare mode
	// where the hostname is auto-discovered from logs.
	PublicHostname string

	mu         sync.Mutex
	cmd        *exec.Cmd
	status     Status
	currentURL string
	startedAt  time.Time
	lastError  string
}

// Status returns a snapshot of the current state. StartedAt is serialised as
// unix millis because Wails' TS generator doesn't understand time.Time.
type Snapshot struct {
	Status     Status `json:"status"`
	URL        string `json:"url"`
	StartedAt  int64  `json:"startedAt"`
	LocalPort  int    `json:"localPort"`
	LastError  string `json:"lastError,omitempty"`
	BinaryPath string `json:"binaryPath"`
	// Mode is "named" when running with a TunnelToken (stable URL,
	// requires Cloudflare account + hostname), otherwise "trycloudflare"
	// (random URL per Start, no account needed).
	Mode string `json:"mode"`
}

func (c *Cloudflared) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	var started int64
	if !c.startedAt.IsZero() {
		started = c.startedAt.UnixMilli()
	}
	mode := "trycloudflare"
	if c.TunnelToken != "" {
		mode = "named"
	}
	return Snapshot{
		Status:     c.status,
		URL:        c.currentURL,
		StartedAt:  started,
		LocalPort:  c.LocalPort,
		LastError:  c.lastError,
		BinaryPath: c.Binary,
		Mode:       mode,
	}
}

// Start launches cloudflared and returns as soon as the process is spawned.
// The discovered public URL is delivered asynchronously via OnURL.
//
// Two command shapes are emitted depending on TunnelToken:
//
//	trycloudflare:  cloudflared tunnel --url http://127.0.0.1:<port> --no-autoupdate
//	named tunnel:   cloudflared tunnel --no-autoupdate run --token <TOKEN>
//
// In named-tunnel mode we don't pass --url because the ingress (and
// therefore the upstream port) is configured in the Cloudflare dashboard
// and baked into the token. The user is responsible for pointing that
// ingress at LocalPort.
func (c *Cloudflared) Start() error {
	c.mu.Lock()
	if c.cmd != nil && c.cmd.Process != nil {
		c.mu.Unlock()
		return errors.New("already running")
	}
	if _, err := os.Stat(c.Binary); err != nil {
		c.status = StatusError
		c.lastError = "cloudflared binary not found: " + err.Error()
		c.mu.Unlock()
		return err
	}
	var cmd *exec.Cmd
	if c.TunnelToken != "" {
		cmd = exec.Command(c.Binary, "tunnel", "--no-autoupdate", "run", "--token", c.TunnelToken)
	} else {
		url := fmt.Sprintf("http://127.0.0.1:%d", c.LocalPort)
		cmd = exec.Command(c.Binary, "tunnel", "--url", url, "--no-autoupdate")
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		c.mu.Unlock()
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		c.mu.Unlock()
		return err
	}
	if err := cmd.Start(); err != nil {
		c.status = StatusError
		c.lastError = err.Error()
		c.mu.Unlock()
		return err
	}
	c.cmd = cmd
	c.status = StatusStarting
	c.startedAt = time.Now()
	c.currentURL = ""
	c.lastError = ""
	// named-tunnel mode knows the public hostname before cloudflared
	// even connects, so we surface it immediately. The connection may
	// still be in-flight, but the URL itself is stable and any client
	// already pointed at it will succeed as soon as cloudflared
	// registers with the Cloudflare edge. If the user forgot to set
	// PublicHostname we leave currentURL empty — the UI will still
	// show "running" via the status flip below, just without a URL.
	announceNamedURL := ""
	if c.TunnelToken != "" {
		c.status = StatusRunning
		if c.PublicHostname != "" {
			announceNamedURL = "https://" + c.PublicHostname
			c.currentURL = announceNamedURL
		}
	}
	c.mu.Unlock()

	if announceNamedURL != "" && c.OnURL != nil {
		c.OnURL(announceNamedURL)
	}

	go c.pipeReader(stdout)
	go c.pipeReader(stderr)

	go func() {
		err := cmd.Wait()
		c.mu.Lock()
		defer c.mu.Unlock()
		if err != nil && c.status != StatusStopped {
			c.status = StatusError
			c.lastError = err.Error()
		}
		if c.status != StatusError {
			c.status = StatusStopped
		}
		c.cmd = nil
		c.currentURL = ""
	}()
	return nil
}

// StartAndWaitURL starts the tunnel and blocks until a public URL is detected
// or the timeout elapses.
func (c *Cloudflared) StartAndWaitURL(timeout time.Duration) (string, error) {
	if err := c.Start(); err != nil {
		return "", err
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		url := c.currentURL
		st := c.status
		c.mu.Unlock()
		if url != "" {
			return url, nil
		}
		if st == StatusError || st == StatusStopped {
			return "", errors.New("cloudflared exited before producing a URL")
		}
		time.Sleep(250 * time.Millisecond)
	}
	return "", errors.New("timeout waiting for cloudflared URL")
}

// Stop terminates the cloudflared process gracefully.
func (c *Cloudflared) Stop() error {
	c.mu.Lock()
	cmd := c.cmd
	c.status = StatusStopped
	c.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	_ = cmd.Process.Signal(interruptSignal())
	done := make(chan struct{})
	go func() { _, _ = cmd.Process.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
	}
	return nil
}

// Rotate stops and restarts the tunnel, returning the new URL.
func (c *Cloudflared) Rotate(timeout time.Duration) (string, error) {
	_ = c.Stop()
	time.Sleep(300 * time.Millisecond)
	return c.StartAndWaitURL(timeout)
}

func (c *Cloudflared) pipeReader(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if c.LogWriter != nil {
			_, _ = c.LogWriter.Write([]byte(line + "\n"))
		}
		// In named-tunnel mode the URL is the user-configured hostname,
		// not something we can scrape from the log. Skip the regex
		// (it would only ever match trycloudflare.com domains anyway).
		if c.TunnelToken != "" {
			continue
		}
		if match := urlRegex.FindString(line); match != "" {
			c.mu.Lock()
			c.currentURL = match
			c.status = StatusRunning
			c.mu.Unlock()
			if c.OnURL != nil {
				c.OnURL(match)
			}
		}
	}
}

func interruptSignal() os.Signal {
	if runtime.GOOS == "windows" {
		return os.Kill
	}
	// syscall.SIGTERM is equivalent to os.Interrupt on unix.
	return os.Interrupt
}
