// Package systemd wraps systemctl --user calls.
// Phase 1: direct exec, readable and trivial to debug.
// Phase 3 planned: replace with D-Bus (go-systemd) for real-time state.
package systemd

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

// Client drives the current user's systemd manager.
type Client struct{}

func (c *Client) command(args ...string) *exec.Cmd {
	return exec.Command("systemctl", append([]string{"--user"}, args...)...)
}

// run executes systemctl, leaving its output visible (useful diagnostics).
func (c *Client) run(args ...string) error {
	slog.Debug("running systemctl", "args", args)
	cmd := c.command(args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("systemctl --user %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// DaemonReload reloads the units after generation.
func (c *Client) DaemonReload() error {
	return c.run("daemon-reload")
}

// RestartWirePlumber restarts wireplumber so freshly deployed configs take effect.
func (c *Client) RestartWirePlumber() error {
	return c.run("restart", "wireplumber")
}

// Start starts a unit (typically a session .target).
func (c *Client) Start(unit string) error {
	return c.run("start", unit)
}

// Stop stops one or more units. Thanks to StopWhenUnneeded=yes in the
// templates, stopping the .target is enough to stop all the instances.
func (c *Client) Stop(units ...string) error {
	return c.run(append([]string{"stop"}, units...)...)
}

// IsActive returns a unit's state ("active", "inactive", "failed"…).
// systemctl is-active exits with a non-zero code when the unit is
// inactive: that's not an error for us.
func (c *Client) IsActive(unit string) string {
	out, _ := c.command("is-active", unit).Output()
	state := strings.TrimSpace(string(out))
	if state == "" {
		state = "unknown"
	}
	return state
}

// ImportGraphicalEnv pushes the current session's graphical environment to
// systemd user, so UIs open on the current Wayland desktop. GNOME/KDE
// already do this at login; belt and braces before every start.
func (c *Client) ImportGraphicalEnv() error {
	var vars []string
	for _, v := range []string{"WAYLAND_DISPLAY", "DISPLAY"} {
		if os.Getenv(v) != "" {
			vars = append(vars, v)
		}
	}
	if len(vars) == 0 {
		return fmt.Errorf("neither WAYLAND_DISPLAY nor DISPLAY in the environment: running outside a graphical session?")
	}
	return c.run(append([]string{"import-environment"}, vars...)...)
}
