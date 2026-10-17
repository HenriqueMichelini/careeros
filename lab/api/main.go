// Command api launches the private local Evaluation Lab on loopback only.
package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	lab "professional-information-repo/internal/evaluationlab"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Evaluation Lab could not start or shut down cleanly.")
		os.Exit(1)
	}
}
func run() error {
	port := flag.Int("port", 0, "loopback port (0 selects an available port)")
	data := flag.String("data-dir", "", "private local history directory")
	assets := flag.String("assets-dir", "lab/web", "local Lab asset directory")
	flag.Parse()
	if *port < 0 || *port > 65535 || flag.NArg() != 0 {
		return fmt.Errorf("invalid launch arguments")
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	provenance, err := lab.CaptureSource(root)
	if err != nil {
		return err
	}
	paths := map[string]string{"test-adapters": "test-adapters/cases.v1.json", "semantic-quality": "semantic-quality/cases.v2.json", "section-review": "section-review/cases.v1.json", "workflow-settings": "workflow-settings/cases.v1.json", "qualification-evidence": "qualification-evidence/cases.json", "application-context": "qualification-evidence/cases.json", "application-baseline": "semantic-quality/controlled.v2.json"}
	fixtures := map[string][]byte{}
	for key, path := range paths {
		raw, readErr := os.ReadFile(filepath.Join(root, "docs", "evaluations", path))
		if readErr != nil {
			return readErr
		}
		fixtures[key] = raw
	}
	planner, err := lab.NewPlanner(fixtures, provenance)
	if err != nil {
		return err
	}
	store, err := lab.OpenFileStore(*data, lab.StoreOptions{})
	if err != nil {
		return err
	}
	defer store.Close()
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		return err
	}
	defer listener.Close()
	origin := "http://" + listener.Addr().String()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	processes, err := lab.NewProcessAdapter(root)
	if err != nil {
		return err
	}
	runner := lab.NewRunner(planner, lab.RunnerOptions{Retain: store.Save, BeforeDispatch: store.Health, Processes: processes})
	var assetFS fs.FS
	if info, statErr := os.Stat(*assets); statErr == nil && info.IsDir() {
		assetFS = os.DirFS(*assets)
	}
	api, err := lab.NewAPI(planner, runner, store, lab.APIOptions{Origin: origin, Context: ctx, Assets: assetFS})
	if err != nil {
		return err
	}
	server := &http.Server{Handler: api, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	stopped := make(chan error, 1)
	go func() { stopped <- server.Serve(listener) }()
	// This URL contains no session or CSRF secret. The local launch shell issues
	// ephemeral authorization in memory after Host/origin checks.
	fmt.Println("Evaluation Lab:", origin+"/lab/")
	select {
	case <-ctx.Done():
	case serveErr := <-stopped:
		if serveErr != nil && serveErr != http.ErrServerClosed {
			cancel()
			return serveErr
		}
	}
	cancel()
	shutdown, finish := context.WithTimeout(context.Background(), 10*time.Second)
	defer finish()
	if err = server.Shutdown(shutdown); err != nil {
		return err
	}
	return api.Shutdown(shutdown)
}
