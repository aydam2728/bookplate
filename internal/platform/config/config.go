// Package config resolves the server configuration from the process
// environment.
//
// Every value has a usable default, so an operator can start BookPlate with no
// configuration at all. Any key may instead be read from a file by setting
// <KEY>_FILE: secrets then arrive through a mounted file rather than through
// the environment, where they are visible to anyone who can run
// "docker inspect".
//
// Nothing in this package reads the environment after Load returns. The
// resolved Config is passed explicitly to whatever needs it.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// Prefix is prepended to every environment key this package reads.
const Prefix = "BOOKPLATE_"

// Config is the fully resolved server configuration.
type Config struct {
	// Addr is the listen address, as accepted by net.Listen.
	Addr string
	// DataDir holds the database and the cover cache. It is the only path an
	// operator has to back up.
	DataDir string
	// LogLevel is the minimum level written to standard output.
	LogLevel slog.Level
}

// Lookup reports the value of an environment key, as os.LookupEnv does. It is
// a parameter so that tests never mutate the real environment.
type Lookup func(key string) (string, bool)

// Load resolves the configuration, reporting every problem it finds rather
// than only the first: an operator fixing a compose file should not have to
// restart once per mistake.
func Load(lookup Lookup) (Config, error) {
	cfg := Config{
		Addr:     ":8080",
		DataDir:  "/data",
		LogLevel: slog.LevelInfo,
	}

	var errs []error
	read := func(key string) string {
		v, err := value(lookup, key)
		if err != nil {
			errs = append(errs, err)
			return ""
		}
		return v
	}

	if v := read("ADDR"); v != "" {
		cfg.Addr = v
	}
	if v := read("DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := read("LOG_LEVEL"); v != "" {
		level, err := parseLevel(v)
		if err != nil {
			errs = append(errs, err)
		} else {
			cfg.LogLevel = level
		}
	}

	if !strings.Contains(cfg.Addr, ":") {
		errs = append(errs, fmt.Errorf(
			"%sADDR: %q is not a listen address, expected a form such as \":8080\"",
			Prefix, cfg.Addr))
	}

	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return cfg, nil
}

// value returns the value of Prefix+key, preferring the contents of the file
// named by Prefix+key+"_FILE" when that is set. Setting both is a mistake
// worth reporting rather than silently resolving.
func value(lookup Lookup, key string) (string, error) {
	name := Prefix + key
	direct, hasDirect := lookup(name)
	path, hasFile := lookup(name + "_FILE")

	switch {
	case hasDirect && hasFile:
		return "", fmt.Errorf("%s and %s_FILE are both set, pick one", name, name)
	case hasFile:
		if strings.TrimSpace(path) == "" {
			return "", fmt.Errorf("%s_FILE: must name a file", name)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("%s_FILE: %w", name, err)
		}
		return strings.TrimSpace(string(content)), nil
	default:
		return strings.TrimSpace(direct), nil
	}
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf(
			"%sLOG_LEVEL: %q is not a level, expected debug, info, warn or error",
			Prefix, s)
	}
}

// FromEnvironment resolves the configuration from the real process
// environment.
func FromEnvironment() (Config, error) {
	return Load(os.LookupEnv)
}
