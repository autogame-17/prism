package tunnel

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// argoSRV is the SRV record cloudflared looks up before it can find a
// Cloudflare edge endpoint to register the tunnel against. If this query
// fails on every resolver we try, there is no point starting the
// subprocess at all — it will just fail with the same i/o timeout we
// already saw in the wild on systems where a TUN proxy hijacks 53/udp.
const argoSRV = "_v2-origintunneld._tcp.argotunnel.com"

// fallbackResolvers are tried in order when the system resolver fails.
// We deliberately keep the list short and well-known: 1.1.1.1 is
// Cloudflare's own resolver (so it will always know about argotunnel),
// 8.8.8.8 is Google, 9.9.9.9 is Quad9. All three answer SRV records
// correctly, which is the actual failure mode on some Chinese ISPs and
// on the 223.5.5.5 resolver Prism users sometimes get pinned to via
// DHCP.
var fallbackResolvers = []string{
	"1.1.1.1:53",
	"8.8.8.8:53",
	"9.9.9.9:53",
}

// pinnedEdgeIPs is the last-resort escape hatch for users whose entire
// 53/udp path is black-holed (typically Clash/Surge/Stash-style TUN
// proxies that route DNS into the proxy and then drop SRV queries).
// When even the public-resolver fallbacks can't answer, we restart
// cloudflared with `--edge <ip:7844>` so it skips the SRV lookup
// entirely and dials the pinned edge IPs directly.
//
// These are Cloudflare's published argotunnel region IPs (region1.v2
// and region2.v2 of argotunnel.com); the address space is
// 198.41.192.0/24 and 198.41.200.0/24 and has been stable for years.
// We hardcode a few from each /24 so a single decommissioned host
// doesn't take the whole tunnel down. cloudflared itself will pick a
// reachable one and retry across the rest.
//
// All of these listen on TCP/UDP 7844 (QUIC + HTTP/2). Source:
// https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/configure-tunnels/cloudflared-parameters/run-parameters/
var pinnedEdgeIPs = []string{
	"198.41.192.7:7844",
	"198.41.192.27:7844",
	"198.41.192.47:7844",
	"198.41.192.67:7844",
	"198.41.192.107:7844",
	"198.41.200.13:7844",
	"198.41.200.33:7844",
	"198.41.200.53:7844",
	"198.41.200.73:7844",
	"198.41.200.113:7844",
}

// PreflightResult captures whether the SRV lookup succeeded and which
// resolver answered. Resolver is empty when the system resolver was
// the one that worked (the common case on healthy networks).
type PreflightResult struct {
	OK       bool
	Resolver string
	Errors   []string
}

// String renders the result for log output.
func (r PreflightResult) String() string {
	if r.OK && r.Resolver == "" {
		return "DNS preflight OK (system resolver)"
	}
	if r.OK {
		return "DNS preflight OK via " + r.Resolver
	}
	return "DNS preflight failed: " + strings.Join(r.Errors, "; ")
}

// preflightDNS probes the argotunnel SRV record using the system
// resolver first, then falls back to a small list of well-known public
// resolvers. The first success wins. Each individual lookup is bounded
// by perResolverTimeout so a single black-holed resolver can't stall
// the whole startup path.
//
// Returning OK=true with a non-empty Resolver is a hint to the caller
// that the OS-level resolver is busted (typically because a proxy
// client's TUN driver is swallowing 53/udp); the caller can surface
// that in the UI so the user knows what to fix.
func preflightDNS(ctx context.Context, perResolverTimeout time.Duration) PreflightResult {
	if perResolverTimeout <= 0 {
		perResolverTimeout = 3 * time.Second
	}

	var errs []string

	if err := lookupSRVWith(ctx, net.DefaultResolver, perResolverTimeout); err == nil {
		return PreflightResult{OK: true}
	} else {
		errs = append(errs, "system: "+err.Error())
	}

	for _, addr := range fallbackResolvers {
		r := newUDPResolver(addr)
		if err := lookupSRVWith(ctx, r, perResolverTimeout); err == nil {
			return PreflightResult{OK: true, Resolver: addr, Errors: errs}
		} else {
			errs = append(errs, addr+": "+err.Error())
		}
	}

	return PreflightResult{OK: false, Errors: errs}
}

func lookupSRVWith(ctx context.Context, r *net.Resolver, timeout time.Duration) error {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	_, addrs, err := r.LookupSRV(cctx, "", "", argoSRV)
	if err != nil {
		return err
	}
	if len(addrs) == 0 {
		return fmt.Errorf("no SRV records returned for %s", argoSRV)
	}
	return nil
}

func newUDPResolver(addr string) *net.Resolver {
	d := &net.Dialer{Timeout: 3 * time.Second}
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			// Force UDP to the chosen public resolver regardless of
			// what the OS thinks the network looks like. We
			// deliberately don't try TCP-fallback here because the
			// failure mode we're trying to diagnose (TUN proxy
			// black-holing 53/udp) usually black-holes 53/tcp too,
			// and we'd rather fail fast and try the next resolver.
			return d.DialContext(ctx, "udp", addr)
		},
	}
}

// FriendlyDNSAdvice translates a failed PreflightResult into a short
// Chinese-language hint suitable for the tunnel error banner. Keeping
// this in the backend means the same string surfaces in the cloudflared
// log panel and in any future system notifications without the
// frontend having to duplicate the heuristics.
func FriendlyDNSAdvice(r PreflightResult) string {
	if r.OK {
		return ""
	}
	joined := strings.ToLower(strings.Join(r.Errors, " "))
	switch {
	case strings.Contains(joined, "i/o timeout"),
		strings.Contains(joined, "no such host"),
		strings.Contains(joined, "server misbehaving"):
		return "DNS 解析失败：cloudflared 无法查询 argotunnel.com 的 SRV 记录。" +
			"通常是代理客户端的 TUN/系统代理拦截了 53/udp，" +
			"请暂时关闭 TUN 增强模式，或在代理规则里让 *.argotunnel.com / *.trycloudflare.com 走直连后重试。"
	}
	return "DNS 解析失败：" + strings.Join(r.Errors, "; ")
}

// preflightOnce coalesces concurrent preflight calls so the UI's
// status-poll loop can't accidentally fire three SRV lookups in
// parallel. We don't memoize success: networks change (laptop wakes
// up, VPN flips on/off), and re-checking is cheap once cached.
var preflightOnce sync.Mutex
