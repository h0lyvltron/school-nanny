// Command school-nanny runs the local homeschool planner and log.
//
// Everything it needs lives in one binary plus one data folder, so the whole
// app can be copied between machines and backed up by copying "data".
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
)

func main() {
	var (
		addr    = flag.String("addr", "127.0.0.1:8080", "address to listen on")
		dataDir = flag.String("data", "", "folder holding the database and uploaded files (default: this computer's app data folder)")
		lan     = flag.Bool("lan", false, "listen on all interfaces so other devices on the home network can connect")
		open    = flag.Bool("open", false, "open the app in a browser once it is listening")
	)
	flag.Parse()

	if *lan {
		_, port, err := net.SplitHostPort(*addr)
		if err != nil {
			log.Fatalf("school nanny: bad -addr %q: %v", *addr, err)
		}
		*addr = ":" + port
	}

	if err := run(*addr, *dataDir, *open); err != nil {
		log.Fatalf("school nanny: %v", err)
	}
}

func run(addr, dataDir string, open bool) error {
	dataDir, adopted, err := prepareDataDir(dataDir)
	if err != nil {
		return err
	}

	store, err := OpenStore(filepath.Join(dataDir, dbFileName))
	if err != nil {
		return err
	}
	defer store.Close()

	if err := store.Migrate(); err != nil {
		return fmt.Errorf("applying migrations: %w", err)
	}

	app, err := NewApp(store, dataDir)
	if err != nil {
		return err
	}

	// Best effort: not being able to write a backup is worth saying out loud,
	// but it is no reason to refuse to open the app.
	if err := app.backupOnStartup(); err != nil {
		log.Printf("could not save a backup: %v", err)
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", addr, err)
	}

	srv := &http.Server{
		Handler:           app.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	url := browserURL(listener.Addr())
	log.Printf("School Nanny is running at %s", url)
	log.Printf("data folder: %s", dataDir)
	if adopted != "" {
		log.Printf("copied your existing records here from %s", adopted)
		log.Printf("that older folder is untouched; delete it once you are happy")
	}
	log.Printf("press Ctrl+C to stop")
	if open {
		go openBrowser(url)
	}

	serveErr := make(chan error, 1)
	go func() {
		err := srv.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serveErr <- err
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-serveErr:
		return err
	case <-stop:
		log.Print("shutting down")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

// prepareDataDir settles where the records live and makes sure the folder is
// there. It returns the folder, plus the older folder its contents came from
// when this was the move to the per-user location.
func prepareDataDir(chosen string) (dir, adoptedFrom string, err error) {
	explicit := chosen != ""
	if !explicit {
		if chosen, err = defaultDataDir(); err != nil {
			return "", "", err
		}
	}
	dir, err = filepath.Abs(chosen)
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", fmt.Errorf("creating %s: %w", dir, err)
	}

	// Asking for a folder outright means meaning it, so nothing is copied in.
	if !explicit {
		if adoptedFrom, err = adoptExistingData(dir); err != nil {
			return "", "", err
		}
	}

	uploads := filepath.Join(dir, uploadsFolderName)
	if err := os.MkdirAll(uploads, 0o755); err != nil {
		return "", "", fmt.Errorf("creating %s: %w", uploads, err)
	}
	return dir, adoptedFrom, nil
}

// browserURL turns a listening address into something a browser can open,
// including the wildcard address used by -lan.
func browserURL(addr net.Addr) string {
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return "http://127.0.0.1:8080"
	}
	if host == "" || host == "::" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
		host = "[" + host + "]"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func openBrowser(url string) {
	time.Sleep(300 * time.Millisecond)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("could not open a browser automatically, visit %s", url)
	}
}
