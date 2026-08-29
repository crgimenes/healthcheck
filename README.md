# healthcheck

Small self-hosted health check service. Periodically checks HTTP endpoints
(expects 200) and pings hosts (partial packet loss counts as unstable, total
loss as down), stores every result in SQLite, and serves an HTML page with the
last week of uptime, one cell per hour.

## Config

Filo file at the path given by `-config`, else `./healthcheck_init.filo`, else
`healthcheck/init.filo` under the platform config dir (`~/Library/Application
Support` on macOS, `$XDG_CONFIG_HOME` on Linux):

```lisp
(set Listen "127.0.0.1:8317")
(set Database "healthcheck.db")
(set Interval "5m")
(check-http "blog" "https://example.com")
(check-ping "gateway" "192.168.1.1" "1m")
```

`check-http` and `check-ping` take a name, a target, and an optional interval
that overrides the global one (default 5m).

## Usage

Run `healthcheck` to start the daemon: it checks each target on its interval
and serves the uptime page on `Listen`. Results older than 30 days are pruned.

`healthcheck -once` runs every check a single time, records the results, prints
them and exits; exit status is non-zero when any check is not up. Suitable for
cron, pointed at the same database the page is served from. `-json` switches
the `-once` output to JSON; `-debug` logs one `key=value` line per result.

`examples/` has a Dockerfile and a ready-made inventory to build and run the
service in a container.
