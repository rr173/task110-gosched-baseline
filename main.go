// Command task110-gosched serves the Go task-scheduling engine HTTP API backed
// by SQLite, and provides a --smoke-test that exercises the full scheduling,
// execution, recovery and stats contract without external dependencies.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"task110-gosched/internal/api"
	"task110-gosched/internal/clock"
	"task110-gosched/internal/metrics"
	"task110-gosched/internal/scheduler"
	"task110-gosched/internal/store"
)

// DefaultAdminToken protects mutating endpoints. Override with GOSCHED_ADMIN_TOKEN.
const DefaultAdminToken = "admin-secret"

func main() {
	smoke := flag.Bool("smoke-test", false, "run self-check and exit")
	dbPath := flag.String("db", "gosched.db", "SQLite database file path")
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	if *smoke {
		if err := runSmokeTest(); err != nil {
			fmt.Println("smoke-test: FAIL:", err)
			osExit(1)
		}
		fmt.Println("smoke-test: ok")
		osExit(0)
	}

	adminToken := os.Getenv("GOSCHED_ADMIN_TOKEN")
	if adminToken == "" {
		adminToken = DefaultAdminToken
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	m := metrics.New()
	sched := scheduler.New(st, clock.RealClock{}, scheduler.DefaultExecutor{}, m, time.Minute)
	if n, err := sched.Recover(); err != nil {
		log.Fatalf("recover: %v", err)
	} else if n > 0 {
		log.Printf("recover: healed %d orphaned runs", n)
	}
	sched.Start()
	defer sched.Stop()

	srv := &http.Server{
		Addr:              *addr,
		Handler:           api.NewMux(sched, st, m, adminToken),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Printf("gosched %s listening on %s (db=%s)", api.Version, *addr, *dbPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

// osExit is indirected so tests can substitute it; in production it is os.Exit.
var osExit = os.Exit
