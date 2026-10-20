// Command api launches the private local Evaluation Lab on loopback only.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	lab "professional-information-repo/internal/evaluationlab"
	"runtime"
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
	buildInfo := flag.Bool("build-info", false, "inspect compiled manifest and local build correspondence without serving or opening history")
	enableGuard := flag.Bool("enable-live-guard", false, "enable controlled guard preflight capability only; no real execution")
	reviewer := flag.String("human-reviewer", "", "explicit local human domain reviewer; approval disabled when empty")
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

	compiled, err := lab.CompiledBuildManifest()
	if err != nil {
		return err
	}
	launch, err := lab.CaptureBuildManifest(root)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	var receipt lab.BuildReceipt
	raw, readErr := os.ReadFile(filepath.Join(filepath.Dir(executable), "lab-build-receipt.json"))
	if readErr == nil {
		if len(raw) > 1<<20 || json.Unmarshal(raw, &receipt) != nil {
			return fmt.Errorf("invalid local build receipt")
		}
	} else if !os.IsNotExist(readErr) {
		return readErr
	}
	digestPath := executable
	if runtime.GOOS == "linux" {
		digestPath = "/proc/self/exe"
	}
	file, err := os.Open(digestPath)
	if err != nil {
		return err
	}
	hash := sha256.New()
	_, err = io.Copy(hash, file)
	file.Close()
	if err != nil {
		return err
	}
	correspondence := lab.CheckBuildCorrespondence(compiled, launch, receipt, hex.EncodeToString(hash.Sum(nil)))
	provenance.Build = &correspondence
	if *buildInfo {
		if err := json.NewEncoder(os.Stdout).Encode(struct {
			Build  lab.BuildCorrespondence `json:"build"`
			Suites []lab.Suite             `json:"suites"`
		}{correspondence, lab.Catalog()}); err != nil {
			return err
		}
	}
	if correspondence.Status == "mismatch" {
		return fmt.Errorf("build correspondence mismatch")
	}
	if *buildInfo {
		return nil
	}
	paths := map[string]string{"test-adapters": "test-adapters/cases.v3.json", "semantic-quality": "semantic-quality/cases.v2.json", "section-review": "section-review/cases.v1.json", "workflow-settings": "workflow-settings/cases.v1.json", "qualification-evidence": "qualification-evidence/cases.json", "application-context": "qualification-evidence/cases.json", "application-baseline": "semantic-quality/controlled.v2.json"}
	fixtures := map[string][]byte{}
	paths["cv-integrity-evidence"] = "cv-lab/evidence.v1.json"
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
	guard, err := lab.NewLiveGuard(planner, store, lab.LiveGuardOptions{Enabled: *enableGuard, HumanReviewer: *reviewer})
	if err != nil {
		return err
	}
	defer guard.Close()
	api, err := lab.NewAPI(planner, runner, store, lab.APIOptions{Origin: origin, Context: ctx, Assets: assetFS, HumanReviewer: *reviewer, LiveGuard: guard})
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
