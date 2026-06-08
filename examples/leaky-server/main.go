// Command leaky-server is a tiny HTTP server that leaks a goroutine per request
// — a realistic target for goroscope.
//
// /work starts a goroutine blocked forever on a channel nobody sends to — the
// classic forgotten-receiver leak.
//
//	go run ./examples/leaky-server
//	curl "http://localhost:8080/work"   # repeat
//	# pprof at http://localhost:6060/debug/pprof/
package main

import (
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof" // registers /debug/pprof on default mux
	"time"
)

// leak blocks forever on a channel that never receives; goroutine can't exit.
func leak(id int) {
	ch := make(chan struct{})
	<-ch
	_ = id // never reached
}

// healthyWork does real work and returns — so the dump isn't 100% leaks.
func healthyWork() {
	time.Sleep(50 * time.Millisecond)
}

func main() {
	var counter int

	mux := http.NewServeMux()

	mux.HandleFunc("/work", func(w http.ResponseWriter, r *http.Request) {
		counter++
		go leak(counter) // the bug: never returns
		healthyWork()
		fmt.Fprintf(w, "did work #%d (and leaked a goroutine)\n", counter)
	})

	mux.HandleFunc("/healthy", func(w http.ResponseWriter, r *http.Request) {
		healthyWork()
		fmt.Fprintln(w, "did work, no leak")
	})

	// pprof on its own port, default mux
	go func() {
		log.Println("pprof listening on http://localhost:6060/debug/pprof/")
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	log.Println("leaky-server listening on http://localhost:8080 (try /work)")
	log.Fatal(http.ListenAndServe("localhost:8080", mux))
}
