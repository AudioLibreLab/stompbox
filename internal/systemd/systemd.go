// Package systemd encapsule les appels à systemctl --user.
// Phase 1 : exec direct, lisible et trivial à déboguer.
// Phase 3 prévue : remplacement par D-Bus (go-systemd) pour les états temps réel.
package systemd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Client pilote le gestionnaire systemd de l'utilisateur courant.
type Client struct{}

func (c *Client) command(args ...string) *exec.Cmd {
	return exec.Command("systemctl", append([]string{"--user"}, args...)...)
}

// run exécute systemctl en laissant sa sortie visible (diagnostics utiles).
func (c *Client) run(args ...string) error {
	cmd := c.command(args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("systemctl --user %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// DaemonReload recharge les units après génération.
func (c *Client) DaemonReload() error {
	return c.run("daemon-reload")
}

// Start démarre une unité (typiquement un .target de session).
func (c *Client) Start(unit string) error {
	return c.run("start", unit)
}

// Stop arrête une ou plusieurs unités. Grâce à StopWhenUnneeded=yes dans les
// gabarits, arrêter le .target suffit à arrêter toutes les instances.
func (c *Client) Stop(units ...string) error {
	return c.run(append([]string{"stop"}, units...)...)
}

// IsActive retourne l'état d'une unité ("active", "inactive", "failed"…).
// systemctl is-active sort en code non nul quand l'unité est inactive :
// ce n'est pas une erreur pour nous.
func (c *Client) IsActive(unit string) string {
	out, _ := c.command("is-active", unit).Output()
	state := strings.TrimSpace(string(out))
	if state == "" {
		state = "unknown"
	}
	return state
}

// ImportGraphicalEnv pousse l'environnement graphique de la session courante
// vers systemd user, pour que les IHM s'ouvrent sur le bureau Wayland actuel.
// GNOME/KDE le font au login ; ceinture et bretelles avant chaque démarrage.
func (c *Client) ImportGraphicalEnv() error {
	var vars []string
	for _, v := range []string{"WAYLAND_DISPLAY", "DISPLAY"} {
		if os.Getenv(v) != "" {
			vars = append(vars, v)
		}
	}
	if len(vars) == 0 {
		return fmt.Errorf("ni WAYLAND_DISPLAY ni DISPLAY dans l'environnement: lancement hors session graphique ?")
	}
	return c.run(append([]string{"import-environment"}, vars...)...)
}
