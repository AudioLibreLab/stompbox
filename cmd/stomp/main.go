// stomp — la pédale du studio. Génère des units systemd user depuis un
// manifeste déclaratif et pilote les sessions audio (Audio as Code).
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/AudioLibreLab/stompbox/internal/manifest"
	"github.com/AudioLibreLab/stompbox/internal/render"
	"github.com/AudioLibreLab/stompbox/internal/systemd"
)

const usage = `stomp — Audio as Code pour studio personnel (PipeWire + systemd user)

Usage: stomp <commande> [options]

Commandes:
  render   Génère les units systemd et fichiers d'env dans un répertoire
  apply    Génère et installe dans ~/.config (+ daemon-reload)
  on       Démarre une session:   stomp on <session>
  off      Arrête une session (ou toutes): stomp off [session]
  status   État des sessions et de leurs apps
  ui       Ouvre l'IHM d'une instance:  stomp ui <instance|kind>

Options communes:
  -f <fichier>   Manifeste (défaut: stompbox.yaml)
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
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "stomp: commande inconnue %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "stomp:", err)
		os.Exit(1)
	}
}

// manifestFlag déclare -f sur un FlagSet et parse les arguments.
func loadManifest(fs *flag.FlagSet, args []string) (*manifest.Manifest, error) {
	path := fs.String("f", "stompbox.yaml", "chemin du manifeste")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	m, err := manifest.Load(*path)
	if err != nil {
		return nil, err
	}
	for _, w := range render.CheckPaths(m) {
		fmt.Fprintln(os.Stderr, "stomp: attention:", w)
	}
	return m, nil
}

func cmdRender(args []string) error {
	fs := flag.NewFlagSet("render", flag.ExitOnError)
	out := fs.String("o", "rendered", "répertoire de sortie")
	m, err := loadManifest(fs, args)
	if err != nil {
		return err
	}
	files, err := render.Render(m)
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := writeFile(filepath.Join(*out, f.Path), f.Content); err != nil {
			return err
		}
		fmt.Println(filepath.Join(*out, f.Path))
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
	for _, f := range files {
		if err := writeFile(filepath.Join(confDir, f.Path), f.Content); err != nil {
			return err
		}
	}
	sd := &systemd.Client{}
	if err := sd.DaemonReload(); err != nil {
		return err
	}
	fmt.Printf("stomp: %d fichiers installés dans %s, units rechargés\n", len(files), confDir)
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
		return fmt.Errorf("session %q inconnue (sessions: %v)", session, m.SessionNames())
	}
	sd := &systemd.Client{}
	// Sécurité Wayland : pousser l'env graphique avant de lancer les IHM.
	if err := sd.ImportGraphicalEnv(); err != nil {
		fmt.Fprintln(os.Stderr, "stomp: attention:", err)
	}
	if err := sd.Start(session + ".target"); err != nil {
		return err
	}
	fmt.Printf("stomp: session %q démarrée 🎸\n", session)
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
		return fmt.Errorf("session %q inconnue (sessions: %v)", fs.Arg(0), m.SessionNames())
	}
	var targets []string
	for _, s := range sessions {
		targets = append(targets, s+".target")
	}
	sd := &systemd.Client{}
	// StopWhenUnneeded=yes dans les gabarits : arrêter les targets
	// suffit à arrêter toutes les instances.
	if err := sd.Stop(targets...); err != nil {
		return err
	}
	fmt.Printf("stomp: arrêté: %v\n", sessions)
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
	switch app.Kind {
	case manifest.KindSooperLooper:
		// -N : ne jamais relancer de moteur, on s'attache à celui de systemd.
		// L'IHM est jetable : la fermer n'interrompt pas les boucles.
		cmd := exec.Command("slgui", "-N", "-P", strconv.Itoa(app.OSCPort))
		if err := cmd.Start(); err != nil {
			return err
		}
		if err := cmd.Process.Release(); err != nil {
			return err
		}
		fmt.Printf("stomp: slgui attaché au moteur %q (port OSC %d)\n", name, app.OSCPort)
		return nil
	case manifest.KindCarla:
		if app.OSCTCPPort == 0 {
			return fmt.Errorf("carla %q: 'osc_tcp_port' requis dans le manifeste pour attacher carla-control", name)
		}
		// L'IHM est jetable : la fermer ne touche pas le host headless.
		cmd := exec.Command("carla-control", fmt.Sprintf("osc.tcp://127.0.0.1:%d/Carla", app.OSCTCPPort))
		if err := cmd.Start(); err != nil {
			return err
		}
		if err := cmd.Process.Release(); err != nil {
			return err
		}
		fmt.Printf("stomp: carla-control attaché au host %q (port OSC TCP %d)\n", name, app.OSCTCPPort)
		return nil
	default:
		return fmt.Errorf("pas d'IHM détachable pour le kind %q", app.Kind)
	}
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
