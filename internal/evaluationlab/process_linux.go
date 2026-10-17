//go:build linux

package evaluationlab

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"professional-information-repo/internal/semanticeval"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

func NewProcessAdapter(root string) (*ProcessAdapter, error) {
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(canonical, "docs/evaluations/test-adapters/cases.v2.json"))
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxImportBytes || semanticeval.Hash(raw) != processFixtureHash {
		return nil, fmt.Errorf("unknown process input manifest")
	}
	var manifest struct {
		Sources map[string]string `json:"sources"`
	}
	if json.Unmarshal(raw, &manifest) != nil || len(manifest.Sources) == 0 || len(manifest.Sources) > 4096 {
		return nil, fmt.Errorf("invalid process sources")
	}
	p := &ProcessAdapter{root: canonical, sources: manifest.Sources, tools: map[string]string{}, capabilities: ProcessCapabilities{RuntimeVersions: map[string]string{}, UnavailableReasons: []string{}}}
	probe, err := os.MkdirTemp("", "careeros-lab-capability-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(probe)
	for _, tool := range []struct {
		name, command string
		args          []string
	}{{"go", "go", []string{"version"}}, {"node", "node", []string{"--version"}}, {"chrome", "google-chrome", []string{"--version"}}, {"bubblewrap", "bwrap", []string{"--version"}}} {
		path, lookupErr := exec.LookPath(tool.command)
		if lookupErr != nil {
			p.capabilities.UnavailableReasons = append(p.capabilities.UnavailableReasons, tool.name+" tool missing")
			continue
		}
		path, lookupErr = filepath.EvalSymlinks(path)
		if lookupErr != nil {
			continue
		}
		p.tools[tool.name] = path
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		cmd := exec.CommandContext(ctx, path, tool.args...)
		cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + probe, "GOTOOLCHAIN=local", "GOENV=off", "GOPROXY=off", "GOSUMDB=off"}
		outputWriter := &processOutput{limit: 4096, cancel: cancel}
		cmd.Stdout = outputWriter
		cmd.Stderr = outputWriter
		versionErr := cmd.Run()
		output := bytes.Clone(outputWriter.data.Bytes())
		cancel()
		if versionErr != nil || outputWriter.truncated {
			p.capabilities.UnavailableReasons = append(p.capabilities.UnavailableReasons, tool.name+" version unavailable")
			continue
		}
		p.capabilities.RuntimeVersions[tool.name] = strings.TrimSpace(string(output))
	}
	if p.tools["bubblewrap"] != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		args := isolationArguments()
		args = append(args, "--json-status-fd", "1", "--", "/usr/bin/true")
		cmd := exec.CommandContext(ctx, p.tools["bubblewrap"], args...)
		cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + probe}
		err = cmd.Run()
		cancel()
		p.capabilities.IsolationAvailable = err == nil
	}
	if p.tools["go"] != "" {
		home, homeErr := os.UserHomeDir()
		if homeErr == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			cmd := exec.CommandContext(ctx, p.tools["go"], "env", "GOMODCACHE")
			cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + home, "GOENV=off", "GOTOOLCHAIN=local", "GOPROXY=off", "GOSUMDB=off"}
			output, cacheErr := cmd.Output()
			cancel()
			if cacheErr == nil && len(output) < 4096 {
				p.tools["module-cache"] = strings.TrimSpace(string(output))
			}
		}
	}
	supervisor, supervisorErr := os.FindProcess(os.Getpid())
	if supervisorErr != nil || !hasProcessHandle(supervisor) {
		p.capabilities.IsolationAvailable = false
		p.capabilities.UnavailableReasons = append(p.capabilities.UnavailableReasons, "stable process handle unavailable; a Go 1.26+ Linux supervisor is required")
	}
	if supervisor != nil {
		supervisor.Release()
	}
	p.capabilities.RuntimeVersions["supervisor"] = runtime.Version()
	if !p.capabilities.IsolationAvailable {
		p.capabilities.UnavailableReasons = append(p.capabilities.UnavailableReasons, "private user/network/PID namespace unavailable; no unsandboxed fallback")
	}

	var document struct {
		CoverageGaps []string `json:"coverageGaps"`
		Cases        []struct {
			ID            string   `json:"id"`
			RequiredTools []string `json:"requiredTools"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(raw, &document); err != nil {
		return nil, err
	}
	p.capabilities.CoverageGaps = document.CoverageGaps
	for _, c := range document.Cases {
		capability := ProcessSelectorCapability{ID: c.ID, RequiredTools: c.RequiredTools, Available: p.capabilities.IsolationAvailable, UnavailableReasons: []string{}}
		if !capability.Available {
			capability.UnavailableReasons = append(capability.UnavailableReasons, "private namespace unavailable")
		}
		for _, tool := range c.RequiredTools {
			if p.tools[tool] == "" || p.capabilities.RuntimeVersions[tool] == "" {
				capability.Available = false
				capability.UnavailableReasons = append(capability.UnavailableReasons, tool+" runtime/version unavailable")
			}
		}
		if command := processCommands[c.ID]; command.unavailable != "" {
			capability.Available = false
			capability.UnavailableReasons = append(capability.UnavailableReasons, command.unavailable)
		}
		p.capabilities.Selectors = append(p.capabilities.Selectors, capability)
	}
	return p, nil
}

func isolationArguments() []string {
	args := []string{"--unshare-all", "--die-with-parent", "--new-session", "--clearenv", "--cap-drop", "ALL", "--proc", "/proc", "--dev", "/dev"}
	for _, path := range []string{"/usr/bin", "/usr/lib", "/usr/lib64", "/usr/share/fonts", "/usr/share/fontconfig", "/etc/fonts", "/etc/ld.so.cache", "/etc/nsswitch.conf"} {
		if _, err := os.Stat(path); err == nil {
			args = append(args, "--ro-bind", path, path)
		}
	}
	return append(args, "--symlink", "usr/bin", "/bin", "--symlink", "usr/lib", "/lib", "--symlink", "usr/lib64", "/lib64")
}

func (p *ProcessAdapter) snapshot(ctx context.Context, destination string) error {
	root, err := os.OpenRoot(p.root)
	if err != nil {
		return err
	}
	defer root.Close()
	var total int64
	for name, expected := range p.sources {
		if err = ctx.Err(); err != nil {
			return err
		}
		if !filepath.IsLocal(name) || filepath.Clean(name) != name || len(name) > 256 {
			return fmt.Errorf("invalid source selector")
		}
		// Reject symlinks in every source component, including approved parents.
		parts := strings.Split(name, "/")
		for i := range parts {
			info, statErr := root.Lstat(filepath.Join(parts[:i+1]...))
			if statErr != nil || info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("unsafe or missing process source")
			}
		}
		file, openErr := root.Open(name)
		if openErr != nil {
			return openErr
		}
		info, statErr := file.Stat()
		if statErr != nil || !info.Mode().IsRegular() || info.Size() > MaxImportBytes {
			file.Close()
			return fmt.Errorf("process source exceeds bounds")
		}
		raw, readErr := io.ReadAll(io.LimitReader(file, MaxImportBytes+1))
		file.Close()
		if readErr != nil || semanticeval.Hash(raw) != expected {
			return fmt.Errorf("reviewed process source changed: %s", name)
		}
		total += int64(len(raw))
		if total > 32<<20 {
			return fmt.Errorf("process source snapshot exceeds bounds")
		}
		path := filepath.Join(destination, name)
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		if err = os.WriteFile(path, raw, 0600); err != nil {
			return err
		}
	}
	return nil
}

type processOutput struct {
	mu         sync.Mutex
	data       bytes.Buffer
	limit      int
	truncated  bool
	cancel     context.CancelFunc
	trackTree  bool
	treeReady  bool
	observe    func(string) error
	observeErr error
}

func (w *processOutput) Write(raw []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := len(raw)
	available := max(0, w.limit-w.data.Len())
	if len(raw) > available {
		w.data.Write(raw[:available])
		w.truncated = true
		w.cancel()
	} else {
		w.data.Write(raw)
	}
	if w.trackTree && !w.treeReady && bytes.Contains(w.data.Bytes(), []byte(`"kind":"lab-probe-tree-ready"`)) {
		w.treeReady = true
		if w.observe != nil {
			if err := w.observe("tree_ready"); err != nil {
				w.observeErr = err
				w.cancel()
			}
		}
	}
	return n, nil
}

func (p *ProcessAdapter) run(ctx context.Context, selector string, maxBytes int64, observe func(string) error) (result ProcessResult, err error) {
	result = ProcessResult{Version: ProcessResultVersion, Selector: selector, Origin: "isolated-go-tests", Outcome: "blocked", RuntimeVersions: cloneJSON(p.capabilities.RuntimeVersions), SourceHash: processFixtureHash, UnavailableReasons: []string{}, CleanupComplete: true}
	defer func() {
		if result.Outcome != "passed" && result.Outcome != "assertion_failed" && !contains(result.UnavailableReasons, result.Outcome) {
			result.UnavailableReasons = append(result.UnavailableReasons, result.Outcome)
		}
	}()
	command, allowed := processCommands[selector]
	if !allowed {
		result.Outcome = "selector_rejected"
		return result, fmt.Errorf("selector is not allowlisted")
	}
	result.Origin = command.origin
	if command.unavailable != "" {
		result.Outcome = command.unavailable
		return result, fmt.Errorf("native print requires separately observed human save evidence")
	}
	if !p.capabilities.IsolationAvailable {
		result.Outcome = "sandbox_spawn_denied"
		result.UnavailableReasons = append(result.UnavailableReasons, p.capabilities.UnavailableReasons...)
		return result, fmt.Errorf("process isolation unavailable")
	}
	if p.tools[command.tool] == "" {
		result.Outcome = "tool_missing"
		return result, fmt.Errorf("required runtime missing")
	}
	p.mu.Lock()
	failed := p.cleanupFailed
	p.mu.Unlock()
	if failed {
		result.Outcome = "cleanup_failed"
		return result, fmt.Errorf("previous process cleanup failed")
	}
	work, err := os.MkdirTemp("", "careeros-lab-process-")
	if err != nil {
		return result, err
	}
	defer func() {
		if result.SpawnAttempted && !result.NamespaceStopped {
			p.mu.Lock()
			p.cleanupFailed = true
			p.mu.Unlock()
			result.CleanupComplete = false
			if result.Outcome != "sandbox_spawn_denied" {
				result.Outcome = "cleanup_failed"
			}
			result.UnavailableReasons = append(result.UnavailableReasons, "owned PID namespace cleanup unavailable; further process attempts disabled")
			err = fmt.Errorf("owned PID namespace cleanup unavailable")
			return
		}
		if cleanupErr := os.RemoveAll(work); cleanupErr != nil {
			p.mu.Lock()
			p.cleanupFailed = true
			p.mu.Unlock()
			result.CleanupComplete = false
			result.Outcome = "cleanup_failed"
			err = cleanupErr
		}
	}()
	source := filepath.Join(work, "workspace")
	if err = os.Mkdir(source, 0700); err != nil {
		return result, err
	}
	if err = p.snapshot(ctx, source); err != nil {
		result.Outcome = "source_changed"
		if errors.Is(ctx.Err(), context.Canceled) {
			result.Outcome = "canceled"
		} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			result.Outcome = "timed_out"
		}
		result.UnavailableReasons = append(result.UnavailableReasons, "approved source snapshot could not be prepared")
		return result, err
	}
	tmp := filepath.Join(work, "tmp")
	if err = os.Mkdir(tmp, 0700); err != nil {
		return result, err
	}
	if err = os.Mkdir(filepath.Join(tmp, "home"), 0700); err != nil {
		return result, err
	}
	args := isolationArguments()
	args = append(args, "--ro-bind", source, "/workspace", "--bind", tmp, "/tmp", "--chdir", "/workspace")
	if command.tool == "go" || command.requiresGo {
		goRoot := filepath.Dir(filepath.Dir(p.tools["go"]))
		if info, statErr := os.Stat(filepath.Join(goRoot, "src/runtime")); statErr != nil || !info.IsDir() {
			result.Outcome = "tool_missing"
			return result, fmt.Errorf("Go distribution unavailable")
		}
		args = append(args, "--ro-bind", goRoot, "/tools/go")
		// Mount only exact local module versions pinned by the reviewed go.mod.
		// The user's full module cache, home and Git credentials remain unmounted.
		for _, module := range []struct{ path, version string }{{"golang.org/x/text", "v0.28.0"}, {"github.com/aws/aws-lambda-go", "v1.49.0"}} {
			name := module.path + "@" + module.version
			local := filepath.Join(p.tools["module-cache"], name)
			info, statErr := os.Lstat(local)
			if p.tools["module-cache"] == "" || statErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				result.Outcome = "dependency_unavailable"
				return result, fmt.Errorf("pinned local Go dependency unavailable")
			}
			args = append(args, "--ro-bind", local, "/tmp/go-mod-cache/"+name)
			for _, extension := range []string{"mod", "info", "ziphash"} {
				relative := filepath.Join("cache/download", module.path, "@v", module.version+"."+extension)
				host := filepath.Join(p.tools["module-cache"], relative)
				if metadata, metadataErr := os.Lstat(host); metadataErr == nil && metadata.Mode().IsRegular() {
					args = append(args, "--ro-bind", host, "/tmp/go-mod-cache/"+filepath.ToSlash(relative))
				}
			}
		}
	}
	if command.tool == "node" {
		args, err = p.nodeRuntime(args, work, source)
		if err != nil {
			result.Outcome = "dependency_unavailable"
			return result, err
		}
	}
	if command.browser {
		if p.tools["chrome"] != "/opt/google/chrome/google-chrome" {
			result.Outcome = "tool_missing"
			return result, fmt.Errorf("supported isolated Chrome runtime unavailable")
		}
		args = append(args, "--ro-bind", "/opt/google/chrome", "/opt/google/chrome", "--symlink", "/opt/google/chrome/google-chrome", "/tools/bin/google-chrome")
	}
	for _, entry := range []struct{ key, value string }{{"PATH", "/tools/bin:/tools/go/bin:/usr/bin:/bin"}, {"HOME", "/tmp/home"}, {"TMPDIR", "/tmp"}, {"GOROOT", "/tools/go"}, {"GOCACHE", "/tmp/go-cache"}, {"GOMODCACHE", "/tmp/go-mod-cache"}, {"GOTOOLCHAIN", "local"}, {"GOENV", "off"}, {"GOPROXY", "off"}, {"GOSUMDB", "off"}, {"CGO_ENABLED", "0"}, {"GOMAXPROCS", "2"}, {"LANG", "C.UTF-8"}} {
		args = append(args, "--setenv", entry.key, entry.value)
	}
	executable := "/tools/go/bin/go"
	if command.tool == "node" {
		executable = "/tools/bin/node"
	}
	args = append(args, "--json-status-fd", "3", "--", executable)
	args = append(args, command.args...)
	childContext, cancel := context.WithCancel(ctx)
	defer cancel()
	output := &processOutput{limit: int(min(128<<10, max(1, maxBytes/4))), cancel: cancel, trackTree: selector == "probe-hang-tree", observe: observe}
	cmd := exec.CommandContext(childContext, p.tools["bubblewrap"], args...)
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	cmd.Stdout = output
	cmd.Stderr = output
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	cmd.WaitDelay = time.Second
	statusReader, statusWriter, pipeErr := os.Pipe()
	if pipeErr != nil {
		return result, pipeErr
	}
	defer statusReader.Close()
	defer statusWriter.Close()
	namespace := watchProcessNamespace(statusReader)
	cmd.ExtraFiles = []*os.File{statusWriter}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		return cmd.Process.Kill()
	}
	if observe != nil {
		if err = observe("intent"); err != nil {
			return result, err
		}
	}
	started := time.Now()
	if err = cmd.Start(); err != nil {
		result.Outcome = "sandbox_spawn_denied"
		return result, err
	}
	statusWriter.Close()
	result.SpawnAttempted = true
	if observe != nil {
		if err = observe("started"); err != nil {
			cancel()
		}
	}
	waitErr := cmd.Wait()
	result.NamespaceStopped = namespace.stop(statusReader)
	result.DurationMS = time.Since(started).Seconds() * 1000
	output.mu.Lock()
	captured := bytes.Clone(output.data.Bytes())
	result.OutputTruncated = output.truncated
	result.TreeReady = output.treeReady
	if err == nil {
		err = output.observeErr
	}
	output.mu.Unlock()
	result.OutputText = strings.ReplaceAll(string(captured), work, "private-run")
	result.OutputHash = semanticeval.Hash([]byte(result.OutputText))
	if cmd.ProcessState != nil {
		code := cmd.ProcessState.ExitCode()
		result.ExitCode = &code
	}
	switch {
	case err != nil:
		result.Outcome = "retention_failed"
	case result.OutputTruncated:
		result.Outcome = "output_limited"
		err = fmt.Errorf("bounded process output exceeded")
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		result.Outcome = "timed_out"
		err = ctx.Err()
	case errors.Is(ctx.Err(), context.Canceled):
		result.Outcome = "canceled"
		err = ctx.Err()
	case waitErr == nil && err == nil:
		result.Outcome = "passed"
	case strings.HasPrefix(result.OutputText, "bwrap:"):
		result.Outcome = "sandbox_spawn_denied"
		err = waitErr
	case cmd.ProcessState != nil && (cmd.ProcessState.ExitCode() < 0 || cmd.ProcessState.ExitCode() >= 128):
		result.Outcome = "crashed"
		err = waitErr
	default:
		result.Outcome = "assertion_failed"
		err = nil
	}
	if selector == "probe-isolation" && !result.OutputTruncated {
		for _, line := range strings.Split(result.OutputText, "\n") {
			var proof struct {
				Kind     string          `json:"kind"`
				Evidence map[string]bool `json:"evidence"`
			}
			if json.Unmarshal([]byte(line), &proof) == nil && proof.Kind == "lab-isolation-probe-v1" {
				result.Evidence = proof.Evidence
				break
			}
		}
	}
	if command.browser && result.NamespaceStopped {
		artifacts, captureErr := captureProcessArtifacts(tmp, command, maxBytes/2, p.capabilities.RuntimeVersions["chrome"])
		result.Artifacts = artifacts
		if captureErr != nil {
			result.UnavailableReasons = append(result.UnavailableReasons, "bounded artifact capture unavailable: "+captureErr.Error())
		}
		result.UnavailableReasons = append(result.UnavailableReasons, "native print/save and per-print-request observations are unavailable in this adapter; CDP PDF bytes are separate controlled evidence")
		result.UnavailableReasons = append(result.UnavailableReasons, "external browser resources are blocked by the private network namespace; this is synthetic isolated browser evidence")
	}
	if result.Outcome != "passed" && result.Outcome != "assertion_failed" {
		result.UnavailableReasons = append(result.UnavailableReasons, result.Outcome)
	}
	return result, err
}

func (p *ProcessAdapter) nodeRuntime(args []string, work, source string) ([]string, error) {
	modules := filepath.Join(p.root, "node_modules")
	info, err := os.Lstat(modules)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("installed Node dependencies unavailable")
	}
	entries, err := os.ReadDir(modules)
	if err != nil || len(entries) > 4096 {
		return nil, fmt.Errorf("Node dependency inventory unavailable")
	}
	staged := filepath.Join(work, "node_modules")
	if err = os.Mkdir(staged, 0700); err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Join(source, "node_modules"), 0700); err != nil {
		return nil, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == ".vite" || strings.HasPrefix(name, ".") && name != ".bin" {
			continue
		}
		if err = os.Symlink("/tools/node_modules/"+name, filepath.Join(staged, name)); err != nil {
			return nil, err
		}
	}
	if err = os.Mkdir(filepath.Join(staged, ".vite"), 0700); err != nil {
		return nil, err
	}
	return append(args, "--ro-bind", modules, "/tools/node_modules", "--bind", staged, "/workspace/node_modules", "--ro-bind", p.tools["node"], "/tools/bin/node"), nil
}
