# Examples

`healthcheck_init.filo` is a basic inventory: crg.eti.br and github.com over
HTTP, plus an ICMP ping to github.com (crg.eti.br does not answer ICMP).

Build and run with Docker from the repository root:

```console
docker build -f examples/Dockerfile -t healthcheck .
docker run --rm -p 8317:8317 -v healthcheck-data:/var/lib/healthcheck healthcheck
```

The uptime page is at http://127.0.0.1:8317/. The named volume keeps the
results across container restarts.

To use the same file outside Docker, change `Listen` to `127.0.0.1:8317` and
`Database` to a writable path, then run
`healthcheck -config examples/healthcheck_init.filo`.
