// Package config reads configuration from the environment.
//
// At step 0.0 only the listen address is required. The other secrets
// (DATABASE_URL, JWT_SECRET, SETTINGS_ENCRYPTION_KEY, BASE_DOMAIN)
// are listed in .env.example and will be wired in on subsequent steps.
package config

import (
	"os"
	"strconv"
)

// Config holds the process configuration.
type Config struct {
	// Host is the listen interface (empty = all interfaces).
	Host string
	// Port is the HTTP server TCP port.
	Port string
	// DatabaseURL is the Postgres connection string (postgres://...).
	DatabaseURL string
	// JWTSecret is the JWT signing key (HS256).
	JWTSecret string
	// SettingsEncryptionKey is the AES-256 key for secrets in the DB (exactly 32 bytes). Empty = secrets forbidden.
	SettingsEncryptionKey string
	// BaseDomain is the public server domain (used as the WebAuthn relying-party ID).
	BaseDomain string
	// StorageRoot is the root data directory on disk (mirror of user trees).
	StorageRoot string
	// VersionKeep is how many file versions to retain (extras are pruned by the GC job).
	VersionKeep int
	// TrashDays is the number of days after which a tombstone is physically removed by the GC job.
	TrashDays int
	// RescanSeconds is the period of the background disk rescan (backstop for fsnotify).
	RescanSeconds int
	// XAccelEnabled: serve files via nginx X-Accel-Redirect (true) or stream directly
	// (false, for deployments without nginx and for client tests). Range requests work out of the box.
	XAccelEnabled bool
}

// Load reads configuration from the environment, applying defaults.
func Load() Config {
	return Config{
		Host:                  os.Getenv("APP_HOST"),
		Port:                  getenv("APP_PORT", "8080"),
		DatabaseURL:           os.Getenv("DATABASE_URL"),
		JWTSecret:             os.Getenv("JWT_SECRET"),
		SettingsEncryptionKey: os.Getenv("SETTINGS_ENCRYPTION_KEY"),
		BaseDomain:            os.Getenv("BASE_DOMAIN"),
		StorageRoot:           getenv("STORAGE_ROOT", "/data"),
		VersionKeep:           getenvInt("VERSION_KEEP", 10),
		TrashDays:             getenvInt("TRASH_DAYS", 30),
		RescanSeconds:         getenvInt("RESCAN_SECONDS", 30),
		XAccelEnabled:         getenvBool("XACCEL_ENABLED", true),
	}
}

// Addr returns the address string for http.Server (host:port).
func (c Config) Addr() string {
	return c.Host + ":" + c.Port
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getenvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
