package config

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
)

const DefaultPort = 8787

type Config struct {
	Enabled        bool
	Port           int
	OpperKey       string
	HMACSecret     string
	CapPerClient   int
	CapGlobal      int
	Origins        []string
	ClientIPHeader string

	GarminAPIKey string
}

func Defaults() Config {
	return Config{Port: DefaultPort, CapPerClient: 20, CapGlobal: 300}
}

func Load(path string) (Config, error) {
	cfg := Defaults()
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return Defaults(), err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		key, value, ok := strings.Cut(text, "=")
		if !ok {
			return Defaults(), fmt.Errorf("%s:%d: expected KEY=VALUE", path, line)
		}
		if err := cfg.set(strings.TrimSpace(key), unquote(strings.TrimSpace(value))); err != nil {
			return Defaults(), fmt.Errorf("%s:%d: %w", path, line, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return Defaults(), err
	}
	if cfg.Enabled && (cfg.OpperKey == "" || cfg.HMACSecret == "" || len(cfg.Origins) == 0) {
		return Defaults(), errors.New("ENABLED=true needs OPPER_API_KEY, HMAC_SECRET and ALLOWED_ORIGINS")
	}
	return cfg, nil
}

func (c *Config) set(key, value string) error {
	var err error
	switch key {
	case "ENABLED":
		c.Enabled, err = strconv.ParseBool(value)
	case "PORT":
		c.Port, err = positive(value)
	case "OPPER_API_KEY":
		c.OpperKey = value
	case "HMAC_SECRET":
		c.HMACSecret = value
	case "DAILY_CAP_PER_CLIENT":
		c.CapPerClient, err = positive(value)
	case "DAILY_CAP_GLOBAL":
		c.CapGlobal, err = positive(value)
	case "ALLOWED_ORIGINS":
		c.Origins = nil
		for _, o := range strings.Split(value, ",") {
			if o = strings.TrimSpace(o); o != "" {
				c.Origins = append(c.Origins, o)
			}
		}
	case "CLIENT_IP_HEADER":
		c.ClientIPHeader = value
	case "GARMIN_API_KEY":
		c.GarminAPIKey = value
	default:
		return fmt.Errorf("unknown key %q", key)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	return nil
}

func positive(value string) (int, error) {
	n, err := strconv.Atoi(value)
	if err == nil && n <= 0 {
		err = errors.New("must be positive")
	}
	return n, err
}

func unquote(value string) string {
	for _, q := range []string{`"`, "'"} {
		if len(value) >= 2 && strings.HasPrefix(value, q) && strings.HasSuffix(value, q) {
			return value[1 : len(value)-1]
		}
	}
	return value
}
