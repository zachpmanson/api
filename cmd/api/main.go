package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zachpmanson/api/internal/tracker"
)

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func main() {
	// Covers SQLite's WAL/SHM and import/backup files as well as the main DB.
	syscall.Umask(0077)
	if err := run(os.Args[1:]); err != nil {
		// Errors are intentionally operational summaries, never request data,
		// driver errors, environment values, credentials or event records.
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	command := "serve"
	if len(args) > 0 && (args[0] == "serve" || args[0] == "import") {
		command, args = args[0], args[1:]
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	dbPath := flags.String("db", env("API_DB_PATH", "./api.sqlite"), "persistent SQLite file")
	listen := flags.String("listen", env("API_LISTEN", "127.0.0.1:3000"), "HTTP listen address (serve)")
	file := flags.String("file", "", "private JSON import file (import only)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return errors.New("invalid command arguments")
	}
	if flags.NArg() != 0 || (command == "import" && *file == "") || (command == "serve" && *file != "") {
		return errors.New("usage: api [serve|import] [-db PATH] [-listen HOST:PORT] [-file JSON]")
	}
	key := os.Getenv("PROD_BROKE_KEY")
	if command == "serve" && key == "" {
		return errors.New("PROD_BROKE_KEY must be set and nonempty")
	}
	var input *os.File
	if command == "import" {
		var err error
		input, err = os.Open(*file)
		if err != nil {
			return errors.New("cannot open import file")
		}
		defer input.Close()
	}
	store, err := tracker.Open(*dbPath)
	if err != nil {
		return errors.New("cannot open or migrate database")
	}
	defer store.Close()
	if command == "import" {
		n, err := store.Import(context.Background(), input)
		if err != nil {
			return err
		}
		fmt.Printf("Import complete: %d new rows; existing matching rows unchanged.\n", n)
		return nil
	}
	handler, err := tracker.Handler(store, key)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:              *listen,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
		ErrorLog:          log.New(io.Discard, "", 0),
	}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		return errors.New("cannot start HTTP listener")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		deadline, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if server.Shutdown(deadline) != nil {
			_ = server.Close()
		}
	}()
	fmt.Fprintln(os.Stderr, "HTTP service started")
	err = server.Serve(listener)
	stop()
	<-shutdownDone
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return errors.New("HTTP service failed")
	}
	return nil
}
