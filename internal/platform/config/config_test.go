package config_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aydam2728/bookplate/internal/platform/config"
)

// envOf builds a Lookup over a fixed map, so no test ever mutates the real
// process environment.
func envOf(values map[string]string) config.Lookup {
	return func(key string) (string, bool) {
		v, ok := values[key]
		return v, ok
	}
}

func TestLoad_WhenEnvironmentIsEmpty_UsesDefaults(t *testing.T) {
	got, err := config.Load(envOf(nil))
	if err != nil {
		t.Fatalf("Load returned an error on an empty environment: %v", err)
	}

	want := config.Config{Addr: ":8080", DataDir: "/data", LogLevel: slog.LevelInfo}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestLoad_WhenValuesAreSet_OverridesDefaults(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want config.Config
	}{
		{
			name: "listen address",
			env:  map[string]string{"BOOKPLATE_ADDR": "127.0.0.1:9000"},
			want: config.Config{Addr: "127.0.0.1:9000", DataDir: "/data", LogLevel: slog.LevelInfo},
		},
		{
			name: "data directory",
			env:  map[string]string{"BOOKPLATE_DATA_DIR": "/srv/bookplate"},
			want: config.Config{Addr: ":8080", DataDir: "/srv/bookplate", LogLevel: slog.LevelInfo},
		},
		{
			name: "log level, case insensitive",
			env:  map[string]string{"BOOKPLATE_LOG_LEVEL": "DEBUG"},
			want: config.Config{Addr: ":8080", DataDir: "/data", LogLevel: slog.LevelDebug},
		},
		{
			name: "warning accepted as warn",
			env:  map[string]string{"BOOKPLATE_LOG_LEVEL": "warning"},
			want: config.Config{Addr: ":8080", DataDir: "/data", LogLevel: slog.LevelWarn},
		},
		{
			name: "surrounding whitespace is ignored",
			env:  map[string]string{"BOOKPLATE_ADDR": "  :9999  "},
			want: config.Config{Addr: ":9999", DataDir: "/data", LogLevel: slog.LevelInfo},
		},
		{
			name: "an empty value falls back to the default",
			env:  map[string]string{"BOOKPLATE_ADDR": ""},
			want: config.Config{Addr: ":8080", DataDir: "/data", LogLevel: slog.LevelInfo},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.Load(envOf(tt.env))
			if err != nil {
				t.Fatalf("Load returned an error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestLoad_WhenFileIndirectionIsUsed_ReadsTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "addr")
	if err := os.WriteFile(path, []byte("127.0.0.1:7000\n"), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	got, err := config.Load(envOf(map[string]string{"BOOKPLATE_ADDR_FILE": path}))
	if err != nil {
		t.Fatalf("Load returned an error: %v", err)
	}
	if got.Addr != "127.0.0.1:7000" {
		t.Errorf("Addr = %q, want %q (the trailing newline must be trimmed)",
			got.Addr, "127.0.0.1:7000")
	}
}

func TestLoad_WhenFileIsMissing_ReportsTheError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent")

	_, err := config.Load(envOf(map[string]string{"BOOKPLATE_ADDR_FILE": path}))
	if err == nil {
		t.Fatal("Load accepted a missing file")
	}
	if !strings.Contains(err.Error(), "BOOKPLATE_ADDR_FILE") {
		t.Errorf("the error does not name the offending key: %v", err)
	}
}

func TestLoad_WhenBothFormsAreSet_RefusesToGuess(t *testing.T) {
	_, err := config.Load(envOf(map[string]string{
		"BOOKPLATE_ADDR":      ":8080",
		"BOOKPLATE_ADDR_FILE": "/anywhere",
	}))
	if err == nil {
		t.Fatal("Load silently picked one of two conflicting sources")
	}
	if !strings.Contains(err.Error(), "pick one") {
		t.Errorf("the error does not say what to do: %v", err)
	}
}

func TestLoad_WhenLogLevelIsUnknown_ReportsIt(t *testing.T) {
	_, err := config.Load(envOf(map[string]string{"BOOKPLATE_LOG_LEVEL": "chatty"}))
	if err == nil {
		t.Fatal("Load accepted an unknown log level")
	}
	if !strings.Contains(err.Error(), "chatty") {
		t.Errorf("the error does not quote the rejected value: %v", err)
	}
}

func TestLoad_WhenSeveralValuesAreInvalid_ReportsAllOfThem(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent")

	_, err := config.Load(envOf(map[string]string{
		"BOOKPLATE_ADDR":          "no-colon-here",
		"BOOKPLATE_LOG_LEVEL":     "chatty",
		"BOOKPLATE_DATA_DIR_FILE": missing,
	}))
	if err == nil {
		t.Fatal("Load accepted three invalid values")
	}

	for _, want := range []string{"BOOKPLATE_ADDR", "BOOKPLATE_DATA_DIR_FILE", "BOOKPLATE_LOG_LEVEL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%s is missing from the report, an operator would restart once per mistake: %v",
				want, err)
		}
	}
}
