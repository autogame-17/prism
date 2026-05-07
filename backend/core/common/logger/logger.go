package logger

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"one-api/common/utils"

	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	loggerINFO  = "INFO"
	loggerWarn  = "WARN"
	loggerError = "ERR"
	loggerDEBUG = "DEBUG"
)
const (
	RequestIdKey = "X-Oneapi-Request-Id"
)

// LogEntry represents a single log entry in memory
type LogEntry struct {
	Timestamp time.Time
	Level     string
	Message   string
}

// LogHistory stores the latest logs in memory
type LogHistory struct {
	entries     []LogEntry
	mutex       sync.RWMutex
	maxSize     int
	subsMu      sync.RWMutex
	subscribers map[int]chan LogEntry
	nextSubID   int
}

// Global log history instance with a default capacity of 500 entries
var logHistory = &LogHistory{
	entries:     make([]LogEntry, 0, 500),
	maxSize:     500,
	subscribers: make(map[int]chan LogEntry),
}

// Subscribe registers a channel that receives each new log entry. The caller
// must drain the channel or risk slowing the logger; Prism uses a buffered
// channel and drops on full. Returns an id and an unsubscribe func.
func (lh *LogHistory) Subscribe(buffer int) (int, <-chan LogEntry, func()) {
	lh.subsMu.Lock()
	defer lh.subsMu.Unlock()
	if buffer <= 0 {
		buffer = 256
	}
	id := lh.nextSubID
	lh.nextSubID++
	ch := make(chan LogEntry, buffer)
	lh.subscribers[id] = ch
	unsub := func() {
		lh.subsMu.Lock()
		defer lh.subsMu.Unlock()
		if c, ok := lh.subscribers[id]; ok {
			delete(lh.subscribers, id)
			close(c)
		}
	}
	return id, ch, unsub
}

// Subscribe is a package-level convenience wrapper around the singleton.
func Subscribe(buffer int) (int, <-chan LogEntry, func()) {
	return logHistory.Subscribe(buffer)
}

// AddEntry adds a new log entry to the history
func (lh *LogHistory) AddEntry(level, message string) {
	lh.mutex.Lock()
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
	}
	lh.entries = append(lh.entries, entry)
	if len(lh.entries) > lh.maxSize {
		lh.entries = lh.entries[1:]
	}
	lh.mutex.Unlock()

	// Fan-out to subscribers; drop on full instead of blocking.
	lh.subsMu.RLock()
	for _, ch := range lh.subscribers {
		select {
		case ch <- entry:
		default:
		}
	}
	lh.subsMu.RUnlock()
}

// GetLatestEntries returns the latest n entries from the log history
func (lh *LogHistory) GetLatestEntries(n int) []LogEntry {
	lh.mutex.RLock()
	defer lh.mutex.RUnlock()

	if n <= 0 || n > len(lh.entries) {
		n = len(lh.entries)
	}

	// Get the latest n entries
	startIdx := len(lh.entries) - n
	if startIdx < 0 {
		startIdx = 0
	}

	// Create a copy of the entries to avoid race conditions
	result := make([]LogEntry, len(lh.entries)-startIdx)
	copy(result, lh.entries[startIdx:])

	return result
}

// GetLatestEntries is a package-level accessor.
func GetLatestEntries(n int) []LogEntry { return logHistory.GetLatestEntries(n) }

var Logger *zap.Logger

var defaultLogDir = "./logs"

func SetupLogger() {
	logDir := getLogDir()
	if logDir == "" {
		return
	}

	writeSyncer := getLogWriter(logDir)

	encoder := getEncoder()

	core := zapcore.NewCore(
		encoder,
		writeSyncer,
		zap.NewAtomicLevelAt(getLogLevel()),
	)
	Logger = zap.New(core, zap.AddCaller())
}

func getEncoder() zapcore.Encoder {
	encodeConfig := zap.NewProductionEncoderConfig()

	encodeConfig.EncodeTime = zapcore.TimeEncoderOfLayout("2006/01/02 - 15:04:05")
	encodeConfig.TimeKey = "time"
	encodeConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	encodeConfig.EncodeCaller = zapcore.ShortCallerEncoder

	encodeConfig.EncodeDuration = zapcore.StringDurationEncoder

	return zapcore.NewConsoleEncoder(encodeConfig)
}

func getLogWriter(logDir string) zapcore.WriteSyncer {
	filename := utils.GetOrDefault("logs.filename", "one-hub.log")
	logPath := filepath.Join(logDir, filename)

	maxsize := utils.GetOrDefault("logs.max_size", 100)
	maxAge := utils.GetOrDefault("logs.max_age", 7)
	maxBackup := utils.GetOrDefault("logs.max_backup", 10)
	compress := utils.GetOrDefault("logs.compress", false)

	lumberJackLogger := &lumberjack.Logger{
		Filename:   logPath,   // 文件位置
		MaxSize:    maxsize,   // 进行切割之前,日志文件的最大大小(MB为单位)
		MaxAge:     maxAge,    // 保留旧文件的最大天数
		MaxBackups: maxBackup, // 保留旧文件的最大个数
		Compress:   compress,  // 是否压缩/归档旧文件
	}

	return zapcore.NewMultiWriteSyncer(zapcore.AddSync(lumberJackLogger), zapcore.AddSync(os.Stderr))
}

func getLogLevel() zapcore.Level {
	logLevel := viper.GetString("log_level")
	switch logLevel {
	case "debug":
		return zap.DebugLevel
	case "info":
		return zap.InfoLevel
	case "warn":
		return zap.WarnLevel
	case "error":
		return zap.ErrorLevel
	case "panic":
		return zap.PanicLevel
	case "fatal":
		return zap.FatalLevel
	default:
		return zap.InfoLevel
	}

}

func getLogDir() string {
	logDir := viper.GetString("log_dir")
	if logDir == "" {
		logDir = defaultLogDir
	}

	var err error
	logDir, err = filepath.Abs(logDir)
	if err != nil {
		log.Fatal(err)
		return ""
	}

	if !utils.IsFileExist(logDir) {
		err = os.Mkdir(logDir, 0777)
		if err != nil {
			log.Fatal(err)
			return ""
		}
	}

	return logDir
}

func SysLog(s string) {
	message := "[SYS] | " + s

	// Add to in-memory log history
	logHistory.AddEntry(loggerINFO, message)

	// Write to log file
	entry := zapcore.Entry{
		Level:   zapcore.InfoLevel,
		Time:    time.Now(),
		Message: message,
	}

	// 使用 Logger 的核心来直接写入日志，绕过等级检查
	if ce := Logger.Core().With([]zapcore.Field{}); ce != nil {
		ce.Write(entry, nil)
	}
}

func SysError(s string) {
	message := "[SYS] | " + s

	// Add to in-memory log history
	logHistory.AddEntry(loggerError, message)

	// Write to log file
	Logger.Error(message)
}

func SysDebug(s string) {
	message := "[SYS] | " + s

	// Add to in-memory log history
	logHistory.AddEntry(loggerDEBUG, message)

	// Write to log file
	Logger.Debug(message)
}

func LogInfo(ctx context.Context, msg string) {
	logHelper(ctx, loggerINFO, msg)
}

func LogWarn(ctx context.Context, msg string) {
	logHelper(ctx, loggerWarn, msg)
}

func LogError(ctx context.Context, msg string) {
	logHelper(ctx, loggerError, msg)
}

func LogDebug(ctx context.Context, msg string) {
	logHelper(ctx, loggerDEBUG, msg)
}

func logHelper(ctx context.Context, level string, msg string) {
	id, ok := ctx.Value(RequestIdKey).(string)
	if !ok {
		id = "unknown"
	}

	logMsg := fmt.Sprintf("%s | %s \n", id, msg)

	// Add to in-memory log history
	logHistory.AddEntry(level, logMsg)

	// Write to log file
	switch level {
	case loggerINFO:
		Logger.Info(logMsg)
	case loggerWarn:
		Logger.Warn(logMsg)
	case loggerError:
		Logger.Error(logMsg)
	case loggerDEBUG:
		Logger.Debug(logMsg)
	default:
		Logger.Info(logMsg)
	}
}

func FatalLog(v ...any) {
	message := fmt.Sprintf("[FATAL] %v | %v \n", time.Now().Format("2006/01/02 - 15:04:05"), v)

	// Add to in-memory log history
	logHistory.AddEntry("FATAL", message)

	// Write to log file
	Logger.Fatal(message)
	// t := time.Now()
	// _, _ = fmt.Fprintf(gin.DefaultErrorWriter, "[FATAL] %v | %v \n", t.Format("2006/01/02 - 15:04:05"), v)
	os.Exit(1)
}

// GetLatestLogs gets the latest n logs from memory
func GetLatestLogs(n int) ([]LogEntry, error) {
	// Get the latest entries from the in-memory log history
	entries := logHistory.GetLatestEntries(n)

	return entries, nil
}
