package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port            string
	DatabaseURL     string
	JWTSecret       string
	RegistrationKey string
	HISBaseURLs     map[string]string
	HISTimeout      time.Duration
}

func Load() (Config, error) { return FromEnv(os.Getenv) }

func FromEnv(get func(string) string) (Config, error) {
	c := Config{Port: get("PORT"), DatabaseURL: get("DATABASE_URL"), JWTSecret: get("JWT_SECRET"), RegistrationKey: get("REGISTRATION_KEY"), HISBaseURLs: make(map[string]string), HISTimeout: 3 * time.Second}
	if c.Port == "" {
		c.Port = "8080"
	}
	p, err := strconv.Atoi(c.Port)
	if err != nil || p < 1 || p > 65535 {
		return c, fmt.Errorf("PORT must be between 1 and 65535")
	}
	u, err := url.Parse(c.DatabaseURL)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Host == "" {
		return c, fmt.Errorf("DATABASE_URL must be a PostgreSQL connection URL")
	}
	for key, value := range map[string]string{"JWT_SECRET": c.JWTSecret, "REGISTRATION_KEY": c.RegistrationKey} {
		lower := strings.ToLower(value)
		if len(value) < 32 || strings.Contains(lower, "change-me") || strings.HasPrefix(lower, "replace") {
			return c, fmt.Errorf("%s must contain at least 32 bytes and must not be a placeholder", key)
		}
	}
	if c.JWTSecret == c.RegistrationKey {
		return c, fmt.Errorf("JWT_SECRET and REGISTRATION_KEY must be different")
	}
	if raw := get("HIS_TIMEOUT"); raw != "" {
		c.HISTimeout, err = time.ParseDuration(raw)
		if err != nil || c.HISTimeout <= 0 || c.HISTimeout > 30*time.Second {
			return c, fmt.Errorf("HIS_TIMEOUT must be a positive duration up to 30s")
		}
	}
	for _, code := range []string{"A", "B"} {
		raw := get("HIS_" + code + "_BASE_URL")
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return c, fmt.Errorf("HIS_%s_BASE_URL must be an HTTP(S) URL without credentials, query or fragment", code)
		}
		c.HISBaseURLs[code] = strings.TrimRight(raw, "/")
	}
	return c, nil
}

func (c Config) Address() string { return net.JoinHostPort("", c.Port) }
