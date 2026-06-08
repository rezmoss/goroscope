# goroscope

**htop for your goroutines.** A live goroutine visualizer and leak detector for any Go process that exposes [`net/http/pprof`](https://pkg.go.dev/net/http/pprof).

`goroscope` polls the target's goroutine dump, groups goroutines by **identical stack**, and tracks each group over time. A leak — the classic "started a goroutine, never stopped it" — shows up as a single group whose count keeps climbing and never comes back down. goroscope flags those groups for you.

```
goroscope  ▸ http://localhost:6060/debug/pprof/goroutine?debug=2
14 goroutines in 1 distinct stack  ▁▂▄█

STATES  chan receive:14

COUNT   Δ      STATE          WHERE
14      +8     chan receive   main.leak  (leaky-server/main.go:25)
```

## Why

`runtime.NumGoroutine()` tells you the count is rising but not *which* goroutines or *where* they're stuck. The raw `/debug/pprof/goroutine?debug=2` dump has that detail but is thousands of lines of noise. goroscope sits in between: it collapses the dump into a handful of stack groups and shows you the one that's growing.

## Install

```sh
go install github.com/rezmoss/goroscope@latest
```

## Use

Your target program needs pprof enabled (most servers already do this):

```go
import _ "net/http/pprof"

go func() { log.Println(http.ListenAndServe("localhost:6060", nil)) }()
```

Then:

```sh
goroscope                    # watch http://localhost:6060 live
goroscope -url host:6060     # a different target
goroscope -snapshot          # print one report and exit (great for CI / logs)
goroscope -interval 500ms    # poll faster
```

In live mode: `↑/↓` select a group, `h/?` help, `q`/`Esc` quit.

## Try it

A deliberately leaky demo server is included:

```sh
go run ./examples/leaky-server      # leaks one goroutine per /work request
# in another shell:
curl "http://localhost:8080/work"   # repeat a few times
goroscope                           # watch main.leak climb
```

## How it works

1. **Collect** — `GET /debug/pprof/goroutine?debug=2` for the full text dump.
2. **Parse** — turn each block into `{id, state, wait, stack frames}`.
3. **Aggregate** — key each goroutine by the ordered list of its frame functions; identical stacks collapse into one group.
4. **Track** — remember each group's first count, so growth (`Δ`) is the leak signal.

No agent, no code change in the target beyond pprof, works against local or remote processes.

## License

MIT
