// stomp — the studio's stompbox. Generates systemd user units from a
// declarative manifest and drives audio sessions (Audio as Code).
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/AudioLibreLab/stompbox/internal/manifest"
	"github.com/AudioLibreLab/stompbox/internal/render"
	"github.com/AudioLibreLab/stompbox/internal/systemd"
)

// version is stamped at build time via -ldflags "-X main.version=…" (see Makefile).
var version = "dev"

const usage = `stomp — Audio as Code for a personal studio (PipeWire + systemd user)

Usage: stomp <command> [options]

Commands:
  render   Generate systemd units and env files into a directory
  apply    Generate and install into ~/.config (+ daemon-reload)
  on       Start a session:   stomp on <session>
  off      Stop a session (or all): stomp off [session]
  status   State of the sessions and their apps
  ui       Open an instance's UI:  stomp ui <instance|kind>
  version  Print the stomp version

Common options:
  -f <file>   Manifest (default: stompbox.yaml)
  -v          Verbose logging (debug level)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch cmd, args := os.Args[1], os.Args[2:]; cmd {
	case "render":
		err = cmdRender(args)
	case "apply":
		err = cmdApply(args)
	case "on":
		err = cmdOn(args)
	case "off":
		err = cmdOff(args)
	case "status":
		err = cmdStatus(args)
	case "ui":
		err = cmdUI(args)
	case "version", "--version":
		fmt.Println(version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "stomp: unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "stomp:", err)
		os.Exit(1)
	}
}

// setupLogging configures the default slog logger. By default only warnings
// and errors are shown; -v enables debug-level diagnostics.
func setupLogging(verbose bool) {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})
	slog.SetDefault(slog.New(h))
}

// loadManifest declares the common -f/-v flags on a FlagSet, parses args,
// configures logging and loads the manifest.
func loadManifest(fs *flag.FlagSet, args []string) (*manifest.Manifest, error) {
	path := fs.String("f", "stompbox.yaml", "manifest path")
	verbose := fs.Bool("v", false, "verbose logging (debug level)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	setupLogging(*verbose)
	slog.Debug("loading manifest", "path", *path)
	m, err := manifest.Load(*path)
	if err != nil {
		return nil, err
	}
	slog.Debug("manifest loaded", "sessions", m.SessionNames())
	for _, w := range render.CheckPaths(m) {
		slog.Warn(w)
	}
	return m, nil
}

func cmdRender(args []string) error {
	fs := flag.NewFlagSet("render", flag.ExitOnError)
	out := fs.String("o", "rendered", "output directory")
	m, err := loadManifest(fs, args)
	if err != nil {
		return err
	}
	files, err := render.Render(m)
	if err != nil {
		return err
	}
	slog.Debug("rendered files", "count", len(files), "output_dir", *out)
	for _, f := range files {
		dst := filepath.Join(*out, f.Path)
		if err := writeFile(dst, f.Content); err != nil {
			return err
		}
		fmt.Println(dst)
	}
	return nil
}

func cmdApply(args []string) error {
	fs := flag.NewFlagSet("apply", flag.ExitOnError)
	m, err := loadManifest(fs, args)
	if err != nil {
		return err
	}
	files, err := render.Render(m)
	if err != nil {
		return err
	}
	confDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	slog.Debug("installing files", "count", len(files), "config_dir", confDir)
	for _, f := range files {
		if err := writeFile(filepath.Join(confDir, f.Path), f.Content); err != nil {
			return err
		}
	}
	sd := &systemd.Client{}
	if err := sd.DaemonReload(); err != nil {
		return err
	}
	if err := sd.RestartWirePlumber(); err != nil {
		slog.Warn("wireplumber restart failed (not running?)", "err", err)
	}
	fmt.Printf("stomp: %d files installed in %s, units reloaded\n", len(files), confDir)
	for _, s := range m.SessionNames() {
		fmt.Printf("  session %-20s → stomp on %s\n", s, s)
	}
	return nil
}

func cmdOn(args []string) error {
	fs := flag.NewFlagSet("on", flag.ExitOnError)
	m, err := loadManifest(fs, args)
	if err != nil {
		return err
	}
	session := fs.Arg(0)
	if session == "" {
		return fmt.Errorf("usage: stomp on <session> (sessions: %v)", m.SessionNames())
	}
	if _, ok := m.Sessions[session]; !ok {
		return fmt.Errorf("unknown session %q (sessions: %v)", session, m.SessionNames())
	}
	sd := &systemd.Client{}
	// Wayland safety: push the graphical env before launching UIs.
	if err := sd.ImportGraphicalEnv(); err != nil {
		slog.Warn(err.Error())
	}
	slog.Debug("starting session", "session", session, "target", session+".target")
	if err := sd.Start(session + ".target"); err != nil {
		return err
	}
	fmt.Printf("stomp: session %q started 🎸\n", session)
	return nil
}

func cmdOff(args []string) error {
	fs := flag.NewFlagSet("off", flag.ExitOnError)
	m, err := loadManifest(fs, args)
	if err != nil {
		return err
	}
	sessions := []string{fs.Arg(0)}
	if fs.Arg(0) == "" {
		sessions = m.SessionNames()
	} else if _, ok := m.Sessions[fs.Arg(0)]; !ok {
		return fmt.Errorf("unknown session %q (sessions: %v)", fs.Arg(0), m.SessionNames())
	}
	var targets []string
	for _, s := range sessions {
		targets = append(targets, s+".target")
	}
	sd := &systemd.Client{}
	// StopWhenUnneeded=yes in the templates: stopping the targets
	// is enough to stop all the instances.
	slog.Debug("stopping targets", "targets", targets)
	if err := sd.Stop(targets...); err != nil {
		return err
	}
	fmt.Printf("stomp: stopped: %v\n", sessions)
	return nil
}

func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	m, err := loadManifest(fs, args)
	if err != nil {
		return err
	}
	sd := &systemd.Client{}
	for _, sName := range m.SessionNames() {
		s := m.Sessions[sName]
		fmt.Printf("%-28s %s\n", sName+".target", sd.IsActive(sName+".target"))
		for _, a := range s.Apps {
			fmt.Printf("  %-26s %s\n", a.Unit(), sd.IsActive(a.Unit()))
		}
		if s.Patchbay != nil {
			unit := "qpwgraph@" + sName + ".service"
			fmt.Printf("  %-26s %s\n", unit, sd.IsActive(unit))
		}
	}
	return nil
}

func cmdUI(args []string) error {
	fs := flag.NewFlagSet("ui", flag.ExitOnError)
	m, err := loadManifest(fs, args)
	if err != nil {
		return err
	}
	name := fs.Arg(0)
	if name == "" {
		return fmt.Errorf("usage: stomp ui <instance|kind> (instances: %v)", m.AppNames())
	}
	app, err := m.ResolveApp(name)
	if err != nil {
		return err
	}
	slog.Debug("resolved instance", "arg", name, "name", app.Name, "kind", app.Kind)
	switch app.Kind {
	case manifest.KindSooperLooper:
		// -N: never spawn a new engine, attach to the one started by systemd.
		// The UI is throwaway: closing it doesn't interrupt the loops.
		cmd := exec.Command("slgui", "-N", "-P", strconv.Itoa(app.OSCPort))
		if err := cmd.Start(); err != nil {
			return err
		}
		if err := cmd.Process.Release(); err != nil {
			return err
		}
		fmt.Printf("stomp: slgui attached to engine %q (OSC port %d)\n", app.Name, app.OSCPort)
		return nil
	case manifest.KindCarla:
		if app.OSCTCPPort == 0 {
			return fmt.Errorf("carla %q: 'osc_tcp_port' required in the manifest to attach carla-control", app.Name)
		}
		// The UI is throwaway: closing it doesn't affect the Carla host.
		cmd := exec.Command("carla-control", fmt.Sprintf("osc.tcp://127.0.0.1:%d/Carla", app.OSCTCPPort))
		if err := cmd.Start(); err != nil {
			return err
		}
		if err := cmd.Process.Release(); err != nil {
			return err
		}
		fmt.Printf("stomp: carla-control attached to host %q (OSC TCP port %d)\n", app.Name, app.OSCTCPPort)
		return nil
	default:
		return fmt.Errorf("no detachable UI for kind %q", app.Kind)
	}
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
