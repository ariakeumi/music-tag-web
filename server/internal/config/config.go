package config

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Config struct {
	Port          string
	MediaRoot     string
	DataDir       string
	DBPath        string
	JWTSecret     string
	PythonBin     string
	TagCLIPath    string
	FPCalcPath    string
	StaticDir     string
	AdminPassword string
	// LoginRequired turns on JWT auth for the API. Default is off: the tool
	// runs on a trusted home LAN and the UI works without a login page.
	LoginRequired bool
}

func Load() *Config {
	c := &Config{}
	c.Port = env("PORT", "8002")
	c.MediaRoot = abs(env("MEDIA_ROOT", "media"))
	c.DataDir = abs(env("DATA_DIR", "data"))
	c.DBPath = abs(env("DB_PATH", filepath.Join(c.DataDir, "db.sqlite3")))
	c.AdminPassword = os.Getenv("ADMIN_PASSWORD")

	c.JWTSecret = os.Getenv("JWT_SECRET")
	if c.JWTSecret == "" {
		c.JWTSecret = os.Getenv("DJANGO_SECRET_KEY")
	}
	if c.JWTSecret == "" {
		// persist a random secret so tokens survive restarts
		secretFile := filepath.Join(c.DataDir, "jwt_secret")
		if b, err := os.ReadFile(secretFile); err == nil && len(b) >= 32 {
			c.JWTSecret = string(b)
		} else {
			buf := make([]byte, 32)
			_, _ = rand.Read(buf)
			c.JWTSecret = hex.EncodeToString(buf)
			_ = os.MkdirAll(c.DataDir, 0o755)
			_ = os.WriteFile(secretFile, []byte(c.JWTSecret), 0o600)
		}
	}

	c.PythonBin = env("PYTHON_BIN", "python3")
	c.TagCLIPath = resolveTagCLI()
	c.FPCalcPath = resolveFPCalc()
	c.StaticDir = abs(env("STATIC_DIR", "static"))
	c.LoginRequired = boolEnv("LOGIN_REQUIRED") || boolEnv("SITE_LOGIN")
	return c
}

func boolEnv(key string) bool {
	switch strings.ToLower(os.Getenv(key)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func abs(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

// resolveTagCLI finds py/tagcli.py relative to the executable, the source tree, or CWD.
func resolveTagCLI() string {
	if v := os.Getenv("TAGCLI_PATH"); v != "" {
		return abs(v)
	}
	candidates := []string{}
	if exe, err := os.Executable(); err == nil {
		base := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(base, "py", "tagcli.py"),
			filepath.Join(filepath.Dir(base), "py", "tagcli.py"),
			filepath.Join(filepath.Dir(filepath.Dir(base)), "server", "py", "tagcli.py"),
		)
	}
	candidates = append(candidates,
		filepath.Join("py", "tagcli.py"),
		filepath.Join("server", "py", "tagcli.py"),
	)
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return abs(p)
		}
	}
	return candidates[0]
}

func resolveFPCalc() string {
	if v := os.Getenv("FPCALC"); v != "" {
		return v
	}
	name := "fpcalc_linux"
	if runtime.GOOS == "darwin" {
		name = "fpcalc"
	}
	if exe, err := os.Executable(); err == nil {
		base := filepath.Dir(exe)
		for _, p := range []string{
			filepath.Join(base, "component", "mz", name),
			filepath.Join(filepath.Dir(base), "component", "mz", name),
			filepath.Join(filepath.Dir(filepath.Dir(base)), "component", "mz", name),
		} {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	// fall back to a system-installed fpcalc (e.g. apt chromaprint)
	if path, err := exec.LookPath("fpcalc"); err == nil {
		return path
	}
	return name
}
