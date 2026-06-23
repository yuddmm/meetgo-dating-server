// Command geonames-import loads cities from a geonames dump into the `cities`
// table (PostGIS). It reads the cities dump (local .txt/.zip or downloads
// cities1000.zip), filters by country / population / settlement feature codes,
// resolves Russian names from alternateNamesV2 (isolanguage=ru), and upserts by
// geoname id.
//
// Usage:
//
//	geonames-import --download --download-alt --country RU --min-pop 1000
//	geonames-import --file cities1000.txt --alt-file alternateNamesV2.txt
//
// DATABASE_URL is read from the environment (or .env).
package main

import (
	"archive/zip"
	"bufio"
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

const (
	citiesURL = "https://download.geonames.org/export/dump/cities1000.zip"
	altURL    = "https://download.geonames.org/export/dump/alternateNamesV2.zip"
)

// Settlement feature codes we keep (cities/towns/admin seats); excludes PPLX
// (city districts), PPLL (localities), historical/abandoned, etc.
var settlementCodes = map[string]bool{
	"PPL": true, "PPLA": true, "PPLA2": true, "PPLA3": true, "PPLA4": true,
	"PPLA5": true, "PPLC": true, "PPLG": true,
}

func main() {
	var (
		file        = flag.String("file", "", "cities dump (.txt/.zip); empty = download cities1000.zip")
		download    = flag.Bool("download", false, "download cities1000.zip")
		altFile     = flag.String("alt-file", "", "alternateNamesV2 dump (.txt/.zip) for Russian names")
		downloadAlt = flag.Bool("download-alt", false, "download alternateNamesV2.zip for Russian names")
		country     = flag.String("country", "RU", "comma-separated ISO country codes (empty = all)")
		minPop      = flag.Int("min-pop", 1000, "minimum population")
		truncate    = flag.Bool("truncate", false, "TRUNCATE cities CASCADE before import (dev re-import)")
	)
	flag.Parse()

	if err := run(*file, *download, *altFile, *downloadAlt, *country, *minPop, *truncate); err != nil {
		log.Fatal(err)
	}
}

type cityRec struct {
	geonameID  int64
	name       string
	country    string
	region     string
	population int
	lat, lng   float64
}

func run(file string, download bool, altFile string, downloadAlt bool, country string, minPop int, truncate bool) error {
	_ = godotenv.Load()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	keep := countrySet(country)

	// 1) Parse cities.
	cities, err := parseCities(file, download, keep, minPop)
	if err != nil {
		return err
	}
	log.Printf("cities parsed: %d", len(cities))

	// 2) Resolve Russian names from alternateNamesV2 (optional but recommended).
	ruNames := map[int64]string{}
	if altFile != "" || downloadAlt {
		ids := make(map[int64]bool, len(cities))
		for _, c := range cities {
			ids[c.geonameID] = true
		}
		if ruNames, err = parseRuNames(altFile, downloadAlt, ids); err != nil {
			return err
		}
		log.Printf("russian names resolved: %d", len(ruNames))
	} else {
		log.Printf("WARNING: no alt-names source — name_ru will be empty (English names only)")
	}

	// 3) Upsert.
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	if truncate {
		if _, err := pool.Exec(ctx, `TRUNCATE cities CASCADE`); err != nil {
			return fmt.Errorf("truncate: %w", err)
		}
		log.Printf("cities truncated (CASCADE)")
	}

	const q = `
		INSERT INTO cities (geoname_id, name, name_ru, country, region, population, location)
		VALUES ($1, $2, $3, $4, $5, $6, ST_SetSRID(ST_MakePoint($7, $8), 4326)::geography)
		ON CONFLICT (geoname_id) DO UPDATE
		SET name = EXCLUDED.name, name_ru = EXCLUDED.name_ru, country = EXCLUDED.country,
		    region = EXCLUDED.region, population = EXCLUDED.population, location = EXCLUDED.location`

	start := time.Now()
	for i, c := range cities {
		if _, err := pool.Exec(ctx, q,
			c.geonameID, c.name, nullable(ruNames[c.geonameID]), c.country, c.region, c.population, c.lng, c.lat); err != nil {
			return fmt.Errorf("upsert geoname %d: %w", c.geonameID, err)
		}
		if (i+1)%2000 == 0 {
			log.Printf("...%d upserted", i+1)
		}
	}
	log.Printf("done: imported %d cities in %s", len(cities), time.Since(start))
	return nil
}

// parseCities reads and filters the cities dump.
func parseCities(file string, download bool, keep map[string]bool, minPop int) ([]cityRec, error) {
	r, cleanup, err := dumpReader(file, download, citiesURL)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	sc := newScanner(r)
	var scanned int
	out := make([]cityRec, 0, 8192)
	for sc.Scan() {
		scanned++
		f := strings.Split(sc.Text(), "\t")
		if len(f) < 15 || f[6] != "P" || !settlementCodes[f[7]] {
			continue
		}
		if keep != nil && !keep[strings.ToUpper(f[8])] {
			continue
		}
		pop, _ := strconv.Atoi(f[14])
		if pop < minPop {
			continue
		}
		lat, e1 := strconv.ParseFloat(f[4], 64)
		lng, e2 := strconv.ParseFloat(f[5], 64)
		id, e3 := strconv.ParseInt(f[0], 10, 64)
		if e1 != nil || e2 != nil || e3 != nil {
			continue
		}
		out = append(out, cityRec{geonameID: id, name: f[1], country: strings.ToUpper(f[8]), region: f[10], population: pop, lat: lat, lng: lng})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan cities: %w", err)
	}
	log.Printf("cities scanned: %d", scanned)
	return out, nil
}

// parseRuNames streams alternateNamesV2 and returns geonameID -> Russian name
// for the given ids (prefers isPreferredName, skips historic).
func parseRuNames(file string, download bool, ids map[int64]bool) (map[int64]string, error) {
	r, cleanup, err := dumpReader(file, download, altURL)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	res := make(map[int64]string, len(ids))
	preferred := make(map[int64]bool, len(ids))
	sc := newScanner(r)
	for sc.Scan() {
		// 0 altId, 1 geonameid, 2 isolanguage, 3 name, 4 isPreferred, 7 isHistoric
		f := strings.Split(sc.Text(), "\t")
		if len(f) < 5 || f[2] != "ru" {
			continue
		}
		id, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil || !ids[id] {
			continue
		}
		if len(f) >= 8 && f[7] == "1" { // skip historic names
			continue
		}
		isPref := f[4] == "1"
		if preferred[id] && !isPref {
			continue // keep the preferred one already stored
		}
		if _, ok := res[id]; ok && !isPref {
			continue // keep first non-preferred until a preferred shows up
		}
		res[id] = f[3]
		if isPref {
			preferred[id] = true
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan alt-names: %w", err)
	}
	return res, nil
}

// dumpReader returns a reader over a geonames TSV from a local file or a download.
// .zip is read from a temp file (memory-safe for large archives like alt-names).
func dumpReader(file string, download bool, url string) (io.Reader, func(), error) {
	path := file
	var tmp string
	if path == "" || download {
		log.Printf("downloading %s ...", url)
		p, err := downloadToTemp(url)
		if err != nil {
			return nil, func() {}, err
		}
		path, tmp = p, p
	}

	cleanRm := func() {
		if tmp != "" {
			_ = os.Remove(tmp)
		}
	}

	if !strings.HasSuffix(strings.ToLower(path), ".zip") {
		fh, err := os.Open(path)
		if err != nil {
			cleanRm()
			return nil, func() {}, err
		}
		return fh, func() { fh.Close(); cleanRm() }, nil
	}

	zr, err := zip.OpenReader(path)
	if err != nil {
		cleanRm()
		return nil, func() {}, fmt.Errorf("open zip: %w", err)
	}
	// Pick the largest .txt entry (the data file, not readme/iso).
	var biggest *zip.File
	for _, zf := range zr.File {
		if strings.HasSuffix(zf.Name, ".txt") && (biggest == nil || zf.UncompressedSize64 > biggest.UncompressedSize64) {
			biggest = zf
		}
	}
	if biggest == nil {
		zr.Close()
		cleanRm()
		return nil, func() {}, fmt.Errorf("no .txt entry in zip")
	}
	rc, err := biggest.Open()
	if err != nil {
		zr.Close()
		cleanRm()
		return nil, func() {}, err
	}
	return rc, func() { rc.Close(); zr.Close(); cleanRm() }, nil
}

func downloadToTemp(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download: status %d", resp.StatusCode)
	}
	f, err := os.CreateTemp("", "geonames-*.zip")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", fmt.Errorf("save download: %w", err)
	}
	f.Close()
	return f.Name(), nil
}

func newScanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 8*1024*1024)
	return sc
}

func countrySet(s string) map[string]bool {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	set := map[string]bool{}
	for _, c := range strings.Split(s, ",") {
		set[strings.ToUpper(strings.TrimSpace(c))] = true
	}
	return set
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
