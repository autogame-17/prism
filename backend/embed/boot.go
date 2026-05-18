package embed

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"

	"one-api/common"
	"one-api/common/config"
	"one-api/common/logger"
	"one-api/common/requester"
	"one-api/middleware"
	"one-api/model"
	"one-api/router"

	"prism/backend/prism/capture"
	"prism/backend/prism/identity"
	"prism/backend/prism/trace"
)

// BootOptions holds runtime configuration for embedding one-hub core.
type BootOptions struct {
	// AppName is used for directory naming (e.g. "Prism").
	AppName string
	// DataDir overrides the data directory (sqlite + config). Empty = auto.
	DataDir string
	// LogDir overrides the log directory. Empty = auto.
	LogDir string
	// ListenAddr is the local listen address for the embedded gin server, e.g. "127.0.0.1:0" for random.
	ListenAddr string
}

// BootResult carries values produced by Boot that the caller needs.
type BootResult struct {
	HTTPAddr string // actual listen address (useful when ListenAddr uses :0)
	DataDir  string
	LogDir   string
	Server   *http.Server
	// TunnelToken is the named-tunnel token read from prism.yaml's
	// `cloudflare.tunnel_token`. Empty means trycloudflare mode.
	TunnelToken string
	// TunnelHostname is the public hostname configured for the named
	// tunnel (e.g. "prism.example.com"). Used only for UI display and
	// the OnURL callback; the actual ingress comes from the token.
	TunnelHostname string
}

// Boot initialises the embedded one-hub core and starts a local gin server.
// Safe to call once per process. Returns when the http server has started
// listening on ListenAddr.
func Boot(opts BootOptions) (*BootResult, error) {
	if opts.AppName == "" {
		opts.AppName = "Prism"
	}
	paths, err := resolvePaths(opts)
	if err != nil {
		return nil, fmt.Errorf("resolve paths: %w", err)
	}

	if err := ensureConfigFile(paths); err != nil {
		return nil, fmt.Errorf("ensure config: %w", err)
	}

	// Point viper at our config file so core's config.InitConf picks it up.
	viper.SetConfigFile(paths.ConfigFile)
	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	// Override critical path-related defaults that core relies on.
	viper.Set("sqlite_path", paths.SQLitePath)
	viper.Set("log_dir", paths.LogDir)
	// Sensible defaults for a desktop single-user deployment.
	if !viper.IsSet("memory_cache_enabled") {
		viper.Set("memory_cache_enabled", false)
	}
	if !viper.IsSet("channel.test_frequency") {
		viper.Set("channel.test_frequency", 0)
	}
	if !viper.IsSet("gin_mode") {
		viper.Set("gin_mode", "release")
	}
	if !viper.IsSet("global.api_rate_limit") {
		viper.Set("global.api_rate_limit", 1800)
	}
	if !viper.IsSet("global.web_rate_limit") {
		viper.Set("global.web_rate_limit", 1000)
	}

	// Run core init sequence (mirror of main.go's startup, minus the bits we don't need).
	config.InitConf()
	logger.SetupLogger()
	if err := common.InitUserToken(); err != nil {
		return nil, fmt.Errorf("init user token: %w", err)
	}

	model.SetupDB()

	if err := trace.Init(); err != nil {
		return nil, fmt.Errorf("init prism trace table: %w", err)
	}

	// Device id underpins the per-request x-request-id (prism-<dev>-<ms>).
	// We persist it in the data dir rather than on the user row because
	// the relevant scope is "this Prism install", not "this account".
	if devID, err := identity.Init(paths.DataDir); err != nil {
		logger.SysError(fmt.Sprintf("init prism device id: %v", err))
	} else {
		logger.SysLog(fmt.Sprintf("prism device id: %s", devID))
	}

	model.InitOptionMap()
	model.NewPricing()
	model.HandleOldTokenMaxId()
	common.InitTokenEncoders()
	requester.InitHttpClient()

	// Prism is a single-user desktop deployment, so the upstream one-hub's
	// per-user pre-quota check is a footgun: out of the box root.Quota is
	// only 1e8, every relay call decrements it, and once it's drained the
	// proxy starts replying 402 "user quota is not enough" even though the
	// user (correctly) configured their token as unlimited. Token unlimited
	// only bypasses the *token* quota — the user-level Quota field is a
	// separate budget on the User row that token unlimited does not waive.
	// On every boot we top the root user back up to ~2e9 so the desktop
	// stops gating its own owner. This is a no-op on hosted deployments
	// because Prism is the only entry point that calls Boot.
	ensureRootUnlimited()

	// Build the HTTP server.
	gin.SetMode(viper.GetString("gin_mode"))
	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(middleware.RequestId())
	middleware.SetUpLogger(engine)

	store := cookie.NewStore([]byte(config.SessionSecret))
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   2592000,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteStrictMode,
	})
	engine.Use(sessions.Sessions("session", store))

	// Every request that actually relays to an upstream model is captured
	// into prism_traces so the user can audit what was sent/received. The
	// middleware self-skips admin/api routes.
	engine.Use(capture.Middleware())

	// Install relay + api routes. We skip web routes (embedded FS) since the
	// desktop UI is served by Wails directly; pass empty FS.
	router.SetApiRouter(engine)
	router.SetDashboardRouter(engine)
	router.SetRelayRouter(engine)

	// Resolve the bind address. Priority:
	//   1. Explicit BootOptions.ListenAddr (used by tests / hosting code)
	//   2. listen_addr in prism.yaml
	//   3. The default 127.0.0.1:39527
	//
	// We deliberately default to a fixed port so external clients (Cursor /
	// curl scripts / IDE extensions) can pin one URL across restarts. If
	// that port is busy we transparently fall back to a random one and log
	// a warning, so a stale Prism / port collision can never block startup.
	listenAddr := opts.ListenAddr
	if listenAddr == "" {
		listenAddr = viper.GetString("listen_addr")
	}
	if listenAddr == "" {
		listenAddr = "127.0.0.1:39527"
	}
	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           engine,
		ReadHeaderTimeout: 15 * time.Second,
	}

	ln, err := listenTCP(listenAddr)
	if err != nil {
		fallback := "127.0.0.1:0"
		logger.SysError(fmt.Sprintf("listen %s failed (%v); falling back to %s", listenAddr, err, fallback))
		ln, err = listenTCP(fallback)
		if err != nil {
			return nil, fmt.Errorf("listen %s: %w", fallback, err)
		}
	}
	srv.Addr = ln.Addr().String()

	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			logger.SysError(fmt.Sprintf("gin serve: %v", err))
		}
	}()

	return &BootResult{
		HTTPAddr:       ln.Addr().String(),
		DataDir:        paths.DataDir,
		LogDir:         paths.LogDir,
		Server:         srv,
		TunnelToken:    viper.GetString("cloudflare.tunnel_token"),
		TunnelHostname: viper.GetString("cloudflare.tunnel_hostname"),
	}, nil
}

// ensureRootUnlimited bumps every RootUser row's Quota to a value high
// enough to never decrement to zero in normal desktop usage. Called from
// Boot after model.SetupDB so the row exists. No-op on a fresh DB where
// createRootAccountIfNeed already inserted the row with this same quota
// (we just keep an existing row from drifting due to old PreQuota debits).
//
// Why "ensure" and not "set once": users on 0.1.0..0.1.2 already have a
// drained Quota in their persisted SQLite, so a one-shot init at first
// boot would not unstick them. Bumping on every boot is cheap (single
// indexed UPDATE) and idempotent.
func ensureRootUnlimited() {
	if model.DB == nil {
		return
	}
	const desktopRootQuota = 2_000_000_000 // headroom of ~2e9 below int32 ceiling
	res := model.DB.Model(&model.User{}).
		Where("role = ?", config.RoleRootUser).
		Where("quota < ?", desktopRootQuota).
		Update("quota", desktopRootQuota)
	if res.Error != nil {
		logger.SysError(fmt.Sprintf("ensure root unlimited: %v", res.Error))
		return
	}
	if res.RowsAffected > 0 {
		logger.SysLog(fmt.Sprintf("desktop mode: lifted %d root user(s) to unlimited quota", res.RowsAffected))
	}
}

// Shutdown attempts to gracefully stop the embedded server and flush the DB.
func Shutdown(res *BootResult) {
	if res != nil && res.Server != nil {
		_ = res.Server.Close()
	}
	model.CloseDB()
}

// --- helpers -----------------------------------------------------------

type appPaths struct {
	DataDir    string
	LogDir     string
	ConfigFile string
	SQLitePath string
}

func resolvePaths(opts BootOptions) (*appPaths, error) {
	dataDir := opts.DataDir
	if dataDir == "" {
		d, err := defaultDataDir(opts.AppName)
		if err != nil {
			return nil, err
		}
		dataDir = d
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}

	logDir := opts.LogDir
	if logDir == "" {
		logDir = filepath.Join(dataDir, "logs")
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, err
	}

	return &appPaths{
		DataDir:    dataDir,
		LogDir:     logDir,
		ConfigFile: filepath.Join(dataDir, "prism.yaml"),
		SQLitePath: filepath.Join(dataDir, "prism.db"),
	}, nil
}

func defaultDataDir(appName string) (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		// Fallback: next to the executable.
		exe, exeErr := os.Executable()
		if exeErr == nil {
			return filepath.Join(filepath.Dir(exe), "."+appName), nil
		}
		return "", err
	}
	if runtime.GOOS == "darwin" {
		// os.UserConfigDir on darwin is ~/Library/Application Support which is fine.
	}
	return filepath.Join(base, appName), nil
}

// ensureConfigFile writes a minimal prism.yaml the first time, generating
// secrets so one-hub's token code doesn't panic.
func ensureConfigFile(p *appPaths) error {
	if _, err := os.Stat(p.ConfigFile); err == nil {
		return nil
	}
	tokenSecret, err := randomHex(16)
	if err != nil {
		return err
	}
	// hashids_salt must have unique characters (sqids alphabet).
	hashSalt, err := uniqueAlphabet(32)
	if err != nil {
		return err
	}
	body := fmt.Sprintf(`# Prism generated configuration. Edit with care.
gin_mode: "release"
log_dir: %q
sqlite_path: %q

user_token_secret: %q
hashids_salt: %q

memory_cache_enabled: false

# Local HTTP server bind address. Keep the port stable so external clients
# (Cursor / IDE extensions) can pin one URL across Prism restarts. Set to
# "127.0.0.1:0" if you want a random ephemeral port instead.
listen_addr: "127.0.0.1:39527"

global:
  api_rate_limit: 1800
  web_rate_limit: 1000

channel:
  test_frequency: 0
`, p.LogDir, p.SQLitePath, tokenSecret, hashSalt)
	return os.WriteFile(p.ConfigFile, []byte(body), 0o600)
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// uniqueAlphabet generates a string of length n using distinct characters,
// suitable for sqids hashids_salt.
func uniqueAlphabet(n int) (string, error) {
	const pool = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	if n > len(pool) {
		n = len(pool)
	}
	buf := []byte(pool)
	// Fisher-Yates shuffle with crypto/rand.
	for i := len(buf) - 1; i > 0; i-- {
		bi, err := randIntN(i + 1)
		if err != nil {
			return "", err
		}
		buf[i], buf[bi] = buf[bi], buf[i]
	}
	return string(buf[:n]), nil
}

func randIntN(n int) (int, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}
	var x uint64
	for i := 0; i < 8; i++ {
		x = x<<8 | uint64(b[i])
	}
	return int(x % uint64(n)), nil
}
