package tunnel

import (
	"bufio"
	"context"
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

	// edgePinned, when true, makes Start() append `--edge <ip:7844>`
	// for each entry in pinnedEdgeIPs so cloudflared bypasses its
	// SRV-based edge discovery entirely. We flip this on inside the
	// retry loop when the DNS preflight tells us the OS can't even
	// resolve argotunnel.com (the classic TUN-proxy black-hole). The
	// flag stays on for the lifetime of the manager so subsequent
	// Rotate() calls keep using the working configuration.
	edgePinned bool
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
	// EdgePinned reports whether cloudflared is running in the
	// fallback "skip DNS, dial pinned edge IPs" mode. This is only
	// true after the DNS preflight failed and we successfully
	// recovered by injecting --edge flags. The UI can use it to
	// show a banner so the user knows their network needs fixing
	// even if the tunnel itself is working.
	EdgePinned bool `json:"edgePinned"`
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
		EdgePinned: c.edgePinned,
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
		args := []string{"tunnel", "--no-autoupdate", "run"}
		// Named-tunnel mode also benefits from the edge-pinned escape
		// hatch when DNS is broken; the `--edge` flag is parsed by
		// the parent `tunnel` command (not `run`), so it has to come
		// before "run" in argv. cloudflared accepts repeated --edge
		// occurrences and will round-robin across them internally.
		if c.edgePinned {
			edgeArgs := []string{}
			for _, ip := range pinnedEdgeIPs {
				edgeArgs = append(edgeArgs, "--edge", ip)
			}
			// Insert edge args after the leading "tunnel" subcommand.
			args = append([]string{"tunnel"}, append(edgeArgs, args[1:]...)...)
		}
		args = append(args, "--token", c.TunnelToken)
		cmd = exec.Command(c.Binary, args...)
	} else {
		args := []string{"tunnel"}
		if c.edgePinned {
			for _, ip := range pinnedEdgeIPs {
				args = append(args, "--edge", ip)
			}
		}
		url := fmt.Sprintf("http://127.0.0.1:%d", c.LocalPort)
		args = append(args, "--url", url, "--no-autoupdate")
		cmd = exec.Command(c.Binary, args...)
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
//
// This is the entry point the UI uses, so it gets the resilience pass:
//
//  1. Run a DNS preflight against argotunnel.com SRV. If even the
//     public-resolver fallbacks can't resolve it, we return early with
//     a friendly message instead of letting cloudflared spawn just to
//     time out 5s later with a stack trace in the log panel.
//  2. If the very first launch produces no URL within `timeout`, retry
//     up to maxAttempts-1 more times with exponential backoff. Each
//     retry calls Stop() first so the wait goroutine has a chance to
//     reap the zombie before we Start() again (Start() refuses if cmd
//     is non-nil).
//
// The overall wall-clock cost is bounded: timeout per attempt + the
// backoff between attempts, capped by maxAttempts. We deliberately do
// NOT extend the timeout when retries kick in — a hung cloudflared
// process won't get healthier with more time, only by being killed and
// restarted.
func (c *Cloudflared) StartAndWaitURL(timeout time.Duration) (string, error) {
	return c.StartAndWaitURLWithRetry(timeout, 3)
}

// StartAndWaitURLWithRetry is the explicit form of StartAndWaitURL.
// maxAttempts <= 0 collapses to a single attempt (no retry), matching
// the historical behaviour for callers that want strict semantics.
func (c *Cloudflared) StartAndWaitURLWithRetry(timeout time.Duration, maxAttempts int) (string, error) {
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	// DNS preflight is meaningful for both trycloudflare and named-
	// tunnel modes: in either case cloudflared has to resolve
	// argotunnel.com on bootstrap. If even the public-resolver
	// fallbacks can't answer, we don't fail — we flip into the
	// edge-pinned escape hatch and retry, because cloudflared's
	// `--edge <ip:7844>` lets it skip the SRV lookup entirely.
	preflightOnce.Lock()
	pctx, pcancel := context.WithTimeout(context.Background(), 12*time.Second)
	res := preflightDNS(pctx, 3*time.Second)
	pcancel()
	preflightOnce.Unlock()
	if c.LogWriter != nil {
		_, _ = c.LogWriter.Write([]byte("[prism] " + res.String() + "\n"))
	}
	if !res.OK {
		// Black-holed DNS: pin the edge IPs and let the retry loop
		// below try again from scratch. We do NOT touch lastError
		// here — Start() will clear it on the next attempt anyway,
		// and lastError carries error-banner semantics which would
		// confusingly stay red in the UI even after recovery. The
		// UI uses Snapshot.EdgePinned (a separate field) to render
		// the "网络受限，已使用边缘 IP 直连" advisory banner.
		c.mu.Lock()
		c.edgePinned = true
		c.mu.Unlock()
		if c.LogWriter != nil {
			_, _ = c.LogWriter.Write([]byte(
				"[prism] " + FriendlyDNSAdvice(res) + "\n" +
					"[prism] DNS 不可用，启用 --edge 边缘 IP 直连模式（pinned " +
					fmt.Sprintf("%d", len(pinnedEdgeIPs)) + " 个 IP）\n"))
		}
	}

	// backoff schedule: 1s, 3s, 7s, ... (doubled+1). Capped at 10s so a
	// pathological maxAttempts can't push the user into a 30s+ wait.
	backoff := time.Second
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		url, err := c.startAndWaitURLOnce(timeout)
		if err == nil {
			return url, nil
		}
		lastErr = err
		// fatal errors that retrying won't fix — bail immediately.
		if errors.Is(err, errAlreadyRunning) || errors.Is(err, errBinaryMissing) {
			return "", err
		}
		// Belt-and-suspenders: if we haven't already flipped to
		// edge-pinned mode (e.g. preflight passed but cloudflared
		// still can't reach the edge — happens when 53/udp works
		// but 443 is blocked, or when the SRV resolver is fine but
		// individual edge hostnames are blackholed), upgrade NOW
		// before the next attempt. This costs nothing if pinned IPs
		// are also unreachable: the next retry just fails the same
		// way, and we surface the original error.
		c.mu.Lock()
		alreadyPinned := c.edgePinned
		c.edgePinned = true
		c.mu.Unlock()
		if !alreadyPinned && c.LogWriter != nil {
			_, _ = c.LogWriter.Write([]byte(
				"[prism] cloudflared 无法连通 Cloudflare 边缘，切换到 --edge 直连模式后重试\n"))
		}
		if attempt == maxAttempts {
			break
		}
		// Make sure the previous attempt's process is fully reaped
		// before we Start() again. startAndWaitURLOnce already calls
		// Stop on its own error path, but we re-Stop here to be
		// defensive in case the process exited on its own (e.g.
		// cloudflared self-quit on a temporary network blip).
		_ = c.Stop()
		c.waitForReap(5 * time.Second)
		if c.LogWriter != nil {
			_, _ = c.LogWriter.Write([]byte(fmt.Sprintf(
				"[prism] tunnel attempt %d/%d failed: %v — retrying in %s\n",
				attempt, maxAttempts, err, backoff,
			)))
		}
		time.Sleep(backoff)
		if backoff < 10*time.Second {
			backoff = backoff*2 + time.Second
			if backoff > 10*time.Second {
				backoff = 10 * time.Second
			}
		}
	}
	return "", lastErr
}

var (
	errAlreadyRunning = errors.New("already running")
	errBinaryMissing  = errors.New("cloudflared binary not found")
)

func (c *Cloudflared) startAndWaitURLOnce(timeout time.Duration) (string, error) {
	if err := c.Start(); err != nil {
		// Normalise the two non-retriable failure modes so the retry
		// loop above can short-circuit. Both are intrinsic to the
		// install, not network conditions.
		msg := err.Error()
		if msg == "already running" {
			return "", errAlreadyRunning
		}
		if len(msg) >= len("cloudflared binary not found") &&
			msg[:len("cloudflared binary not found")] == "cloudflared binary not found" {
			return "", errBinaryMissing
		}
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
			// Stop already happened on the wait goroutine; surface
			// the error captured there if we have one.
			c.mu.Lock()
			le := c.lastError
			c.mu.Unlock()
			if le == "" {
				le = "cloudflared exited before producing a URL"
			}
			return "", errors.New(le)
		}
		time.Sleep(250 * time.Millisecond)
	}
	// Timeout: kill the in-flight process so the next retry's Start()
	// doesn't bounce off "already running".
	_ = c.Stop()
	return "", errors.New("timeout waiting for cloudflared URL")
}

// waitForReap blocks until the wait goroutine has cleared c.cmd, or
// until timeout elapses. It mirrors the polling loop in Rotate(); we
// keep them duplicated rather than extracted so each call site stays
// obvious about why it's waiting.
func (c *Cloudflared) waitForReap(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		gone := c.cmd == nil
		c.mu.Unlock()
		if gone {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
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
//
// Stop() sends SIGTERM but the wait goroutine clearing c.cmd back to nil
// is what really gates a clean restart — Start() guards on `c.cmd != nil`
// and would otherwise return "already running" if we race the wait
// goroutine. Poll until the wait goroutine has reaped the process (or
// give up after 5s, which means cloudflared ignored SIGTERM and
// Stop()'s 3s Kill path is still in flight).
func (c *Cloudflared) Rotate(timeout time.Duration) (string, error) {
	_ = c.Stop()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		gone := c.cmd == nil
		c.mu.Unlock()
		if gone {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
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
