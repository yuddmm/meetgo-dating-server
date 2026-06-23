// Command geonames-import loads cities from a geonames dump into the `cities`
// table (PostGIS). It reads a local TSV file or downloads cities1000.zip, filters
// by country and population, and upserts by geoname id.
//
// Usage:
//
//	geonames-import --file cities1000.txt --country RU --min-pop 1000
//	geonames-import --download --country RU --min-pop 1000
//
// DATABASE_URL is read from the environment (or .env).
package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

const citiesURL = "https://download.geonames.org/export/dump/cities1000.zip"

func main() {
	var (
		file     = flag.String("file", "", "path to a geonames dump (.txt or .zip); empty = download cities1000.zip")
		download = flag.Bool("download", false, "download cities1000.zip from geonames")
		country  = flag.String("country", "RU", "comma-separated ISO country codes to keep (empty = all)")
		minPop   = flag.Int("min-pop", 1000, "minimum population")
	)
	flag.Parse()

	if err := run(*file, *download, *country, *minPop); err != nil {
		log.Fatal(err)
	}
}

func run(file string, download bool, country string, minPop int) error {
	_ = godotenv.Load()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	rc, err := openDump(file, download)
	if err != nil {
		return err
	}
	defer rc.Close()

	keep := countrySet(country)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	return importDump(ctx, pool, rc, keep, minPop)
}

// openDump returns a reader over the geonames TSV (handling .zip and download).
func openDump(file string, download bool) (io.ReadCloser, error) {
	if file == "" || download {
		log.Printf("downloading %s ...", citiesURL)
		resp, err := http.Get(citiesURL)
		if err != nil {
			return nil, fmt.Errorf("download: %w", err)
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read download: %w", err)
		}
		return zipEntryReader(raw)
	}
	if strings.HasSuffix(strings.ToLower(file), ".zip") {
		raw, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		return zipEntryReader(raw)
	}
	return os.Open(file)
}

// zipEntryReader returns the first .txt entry inside a zip archive.
func zipEntryReader(raw []byte) (io.ReadCloser, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, ".txt") {
			return f.Open()
		}
	}
	return nil, fmt.Errorf("no .txt entry in zip")
}

func countrySet(s string) map[string]bool {
	if strings.TrimSpace(s) == "" {
		return nil // nil = keep all
	}
	set := map[string]bool{}
	for _, c := range strings.Split(s, ",") {
		set[strings.ToUpper(strings.TrimSpace(c))] = true
	}
	return set
}

// importDump parses the geonames TSV and upserts matching cities.
func importDump(ctx context.Context, pool *pgxpool.Pool, r io.Reader, keep map[string]bool, minPop int) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 4*1024*1024) // geonames lines can be long

	const q = `
		INSERT INTO cities (geoname_id, name, name_ru, country, region, population, location)
		VALUES ($1, $2, $3, $4, $5, $6, ST_SetSRID(ST_MakePoint($7, $8), 4326)::geography)
		ON CONFLICT (geoname_id) DO UPDATE
		SET name = EXCLUDED.name, name_ru = EXCLUDED.name_ru, country = EXCLUDED.country,
		    region = EXCLUDED.region, population = EXCLUDED.population, location = EXCLUDED.location`

	var scanned, inserted int
	start := time.Now()
	batch := pool // simple per-row exec; the filtered RU set is small
	for sc.Scan() {
		scanned++
		f := strings.Split(sc.Text(), "\t")
		if len(f) < 15 {
			continue
		}
		if f[6] != "P" { // feature class: populated place
			continue
		}
		if keep != nil && !keep[strings.ToUpper(f[8])] { // country code
			continue
		}
		pop, _ := strconv.Atoi(f[14])
		if pop < minPop {
			continue
		}
		lat, err1 := strconv.ParseFloat(f[4], 64)
		lng, err2 := strconv.ParseFloat(f[5], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		geonameID, err := strconv.ParseInt(f[0], 10, 64)
		if err != nil {
			continue
		}
		nameIntl, nameRu := names(f[1], f[2], f[3])

		if _, err := batch.Exec(ctx, q,
			geonameID, nameIntl, nullable(nameRu), strings.ToUpper(f[8]), f[10], pop, lng, lat); err != nil {
			return fmt.Errorf("upsert geoname %d: %w", geonameID, err)
		}
		inserted++
		if inserted%2000 == 0 {
			log.Printf("...%d cities", inserted)
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	log.Printf("done: scanned %d, imported %d cities in %s", scanned, inserted, time.Since(start))
	return nil
}

// names resolves the international name and a best-effort Russian name.
// geonames has no language tag in the main dump, so name_ru = the name if it is
// Cyrillic, else the first Cyrillic alternate name; international = romanized.
func names(name, ascii, alternates string) (intl, ru string) {
	if isCyrillic(name) {
		intl = ascii
		if intl == "" {
			intl = name
		}
		return intl, name
	}
	intl = name
	for _, alt := range strings.Split(alternates, ",") {
		if isCyrillic(alt) {
			return intl, alt
		}
	}
	return intl, ""
}

func isCyrillic(s string) bool {
	for _, r := range s {
		if r >= 0x0400 && r <= 0x04FF {
			return true
		}
	}
	return false
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
