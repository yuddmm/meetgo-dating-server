// Package geoip resolves a client IP to an ISO country code using a local
// MaxMind/DB-IP .mmdb database (offline, no external calls). When no database
// is configured it degrades to a no-op resolver that returns "" (unknown).
package geoip

import (
	"fmt"
	"net"

	"github.com/oschwald/geoip2-golang"
)

// GeoIP resolves IPs to country codes. A zero/disabled instance (no db) always
// returns "".
type GeoIP struct {
	db *geoip2.Reader
}

// New opens the mmdb at path. An empty path returns a disabled resolver (no
// error) so the app runs without a database in development.
func New(path string) (*GeoIP, error) {
	if path == "" {
		return &GeoIP{}, nil
	}
	db, err := geoip2.Open(path)
	if err != nil {
		return nil, fmt.Errorf("geoip: open %q: %w", path, err)
	}
	return &GeoIP{db: db}, nil
}

// Enabled reports whether a database is loaded.
func (g *GeoIP) Enabled() bool { return g.db != nil }

// Country returns the ISO-3166 alpha-2 code (e.g. "RU") for the IP, or "" when
// unknown / disabled / unparseable.
func (g *GeoIP) Country(ip string) string {
	if g.db == nil {
		return ""
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ""
	}
	rec, err := g.db.Country(parsed)
	if err != nil {
		return ""
	}
	return rec.Country.IsoCode
}

// Close releases the underlying database.
func (g *GeoIP) Close() error {
	if g.db != nil {
		return g.db.Close()
	}
	return nil
}
