package config

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	_ "github.com/kardianos/minwinsvc" // import minwinsvc for windows services
	"github.com/rs/zerolog/log"
	"gopkg.in/ini.v1"
)

var (
	// Path of config file
	Path string

	isWindows   bool
	appWorkPath string
)

// General defines the general configuration.
type General struct {
	WatchFile    string `ini:"WATCH_FILE"`
	Root         string `ini:"ROOT"`
	RestartDelay int    `ini:"RESTART_DELAY"`
}

// Backend defines the api backend configuration
type Backend struct {
	URL      string `ini:"URL"`
	User     string `ini:"USER"`
	Password string `ini:"PASSWORD"`
}

// Log defines the logging configuration.
type Log struct {
	Level   string `ini:"LEVEL"`
	Colored bool   `ini:"COLORED"`
	Pretty  bool   `ini:"PRETTY"`
}

// Config defines the general configuration.
type Config struct {
	General General
	Backend Backend
	Log     Log
}

// New prepares a new default configuration.
func New() (*Config, error) {
	if !filepath.IsAbs(Path) {
		Path = path.Join(appWorkPath, Path)
	}

	cfg, err := ini.Load(Path)
	if err != nil {
		return nil, err
	}

	generalCfg := new(General)
	if err = cfg.Section("general").MapTo(generalCfg); err != nil {
		return nil, err
	}

	if !filepath.IsAbs(generalCfg.Root) {
		generalCfg.Root = path.Join(appWorkPath, generalCfg.Root)
	}
	if err := os.MkdirAll(generalCfg.Root, os.ModePerm); err != nil {
		return nil, err
	}

	backendCfg := new(Backend)
	if err = cfg.Section("backend").MapTo(backendCfg); err != nil {
		return nil, err
	}

	backendCfg.URL = strings.TrimSuffix(backendCfg.URL, "/")

	logCfg := new(Log)
	if err = cfg.Section("log").MapTo(logCfg); err != nil {
		return nil, err
	}

	return &Config{
		General: *generalCfg,
		Backend: *backendCfg,
		Log:     *logCfg,
	}, nil
}

func init() {
	isWindows = runtime.GOOS == "windows"

	var appPath string
	var err error
	if appPath, err = getAppPath(); err != nil {
		log.Fatal().
			Err(err).
			Msg("AppPath could not be found")

		os.Exit(1)
	}

	appWorkPath = getWorkPath(appPath)
}

func getAppPath() (string, error) {
	var appPath string
	var err error

	if isWindows && filepath.IsAbs(os.Args[0]) {
		appPath = filepath.Clean(os.Args[0])
	} else {
		appPath, err = exec.LookPath(os.Args[0])
	}

	if err != nil {
		return "", err
	}
	appPath, err = filepath.Abs(appPath)
	if err != nil {
		return "", err
	}

	// Note: we don't use path.Dir here because it does not handle case
	//		 which path starts with two "/" in Windows: "//psf/Home/..."
	return strings.Replace(appPath, "\\", "/", -1), err
}

func getWorkPath(appPath string) string {
	workPath := ""

	i := strings.LastIndex(appPath, "/")
	if i == -1 {
		workPath = appPath
	} else {
		workPath = appPath[:i]
	}

	// Note: we don't use path.Dir here because it does not handle case
	//		 which path starts with two "/" in Windows: "//psf/Home/..."
	return strings.Replace(workPath, "\\", "/", -1)
}
