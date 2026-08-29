package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"text/tabwriter"
	"time"
)

var Version = "dev"

const (
	retentionDays   = 30
	shutdownTimeout = 5 * time.Second
)

func usage() {
	fmt.Printf(`healthcheck %s - periodic HTTP and ping checks with a weekly uptime page

Usage:
  healthcheck [flags]

Runs each check on its interval (default %s), stores results in SQLite and
serves an HTML page with the last week of uptime. Results older than %d days
are pruned. With -once it runs every check a single time, records the results
and exits; exit status is non-zero when any check is not up (cron friendly).

Flags:
  -config PATH  read the Filo config from PATH
  -once         run all checks once, print results to stdout and exit
  -json         with -once, print results as JSON
  -debug        log one key=value line per check result to stderr
  -h            show this help

Config: ./healthcheck_init.filo, else healthcheck/init.filo under the platform
config dir (~/Library/Application Support on macOS, $XDG_CONFIG_HOME on Linux)

  (set Listen "127.0.0.1:8317")
  (set Database "healthcheck.db")
  (set Interval "5m")
  (check-http "blog" "https://example.com")
  (check-ping "gateway" "192.168.1.1" "1m")
`, Version, defaultInterval, retentionDays)
}

func main() {
	flag.Usage = usage
	configFile := flag.String("config", "", "path to the Filo config file")
	once := flag.Bool("once", false, "run all checks once and exit")
	asJSON := flag.Bool("json", false, "with -once, print results as JSON")
	debug := flag.Bool("debug", false, "log each check result to stderr")
	flag.Parse()

	os.Exit(run(*configFile, *once, *asJSON, *debug))
}

func run(configFile string, once, asJSON, debug bool) int {
	if configFile == "" {
		configFile = configPath()
	}
	cfg, err := loadConfig(configFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	store, err := openStore(cfg.Database)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = store.Close() }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if once {
		return runOnce(ctx, cfg, store, asJSON)
	}
	return runDaemon(ctx, cfg, store, debug)
}

func runOnce(ctx context.Context, cfg *Config, store *Store, asJSON bool) int {
	results := make([]result, len(cfg.Checks))
	var wg sync.WaitGroup
	for i, c := range cfg.Checks {
		wg.Go(func() {
			results[i] = runCheck(ctx, c)
		})
	}
	wg.Wait()

	code := 0
	for _, r := range results {
		err := store.insert(r)
		if err != nil {
			fmt.Fprintf(os.Stderr, "store %s: %v\n", r.Service, err)
			code = 1
		}
		if r.Status != statusUp {
			code = 1
		}
	}

	if asJSON {
		err := json.NewEncoder(os.Stdout).Encode(results)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return code
	}

	tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	for _, r := range results {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%dms\t%s\n", r.Service, r.Status, r.LatencyMS, r.Detail)
	}
	err := tw.Flush()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return code
}

func watch(ctx context.Context, c Check, store *Store, debug bool) {
	t := time.NewTicker(c.Interval)
	defer t.Stop()
	for {
		r := runCheck(ctx, c)
		if ctx.Err() != nil {
			return
		}
		err := store.insert(r)
		if err != nil {
			log.Printf("store %s: %v", c.Name, err)
		}
		if debug {
			log.Printf("service=%s kind=%s status=%s latency_ms=%d detail=%q", r.Service, c.Kind, r.Status, r.LatencyMS, r.Detail)
		}

		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func pruneLoop(ctx context.Context, store *Store, debug bool) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		n, err := store.prune(time.Now().UTC().AddDate(0, 0, -retentionDays))
		if err != nil {
			log.Printf("prune: %v", err)
		}
		if debug && n > 0 {
			log.Printf("prune: removed=%d", n)
		}

		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func runDaemon(ctx context.Context, cfg *Config, store *Store, debug bool) int {
	var wg sync.WaitGroup
	for _, c := range cfg.Checks {
		wg.Go(func() {
			watch(ctx, c, store, debug)
		})
	}
	wg.Go(func() {
		pruneLoop(ctx, store, debug)
	})

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           newWebHandler(cfg, store),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}
	go func() { // #nosec G118 -- runs after ctx is canceled; Shutdown needs a fresh deadline
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()

	log.Printf("healthcheck %s: %d checks, page on http://%s", Version, len(cfg.Checks), cfg.Listen)
	err := srv.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	wg.Wait()
	return 0
}
