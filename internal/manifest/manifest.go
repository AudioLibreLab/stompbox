// Package manifest loads and validates the declarative stompbox.yaml
// manifest, the source of truth for the studio's sessions.
package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"gopkg.in/yaml.v3"
)

// Kinds supported by the generator.
const (
	KindCarla        = "carla"
	KindHydrogen     = "hydrogen"
	KindSooperLooper = "sooperlooper"
)

// Manifest is the root of the stompbox.yaml file.
type Manifest struct {
	Sessions map[string]*Session `yaml:"sessions"`

	// Dir is the absolute directory containing the manifest;
	// relative paths in the manifest are resolved against it.
	Dir string `yaml:"-"`
}

// Session describes a set of applications and their wiring.
type Session struct {
	Apps     []*App    `yaml:"apps"`
	Patchbay *Patchbay `yaml:"patchbay"`
}

// App describes an application instance. Fields are kind-specific;
// validation enforces which ones are required.
type App struct {
	Kind string `yaml:"kind"`
	Name string `yaml:"name"`

	// carla
	Preset     string `yaml:"preset"`
	OSCTCPPort int    `yaml:"osc_tcp_port"`

	// hydrogen
	Song string `yaml:"song"`

	// sooperlooper
	OSCPort     int    `yaml:"osc_port"`
	Loops       int    `yaml:"loops"`
	Channels    int    `yaml:"channels"`
	LoopTime    int    `yaml:"looptime"`
	Session     string `yaml:"session"`
	MIDIBinding string `yaml:"midi_binding"`
}

// Patchbay describes a session's qpwgraph patchbay file.
type Patchbay struct {
	File      string `yaml:"file"`
	Exclusive bool   `yaml:"exclusive"`
}

// Unit returns the instance's systemd unit name (e.g. carla@clean.service).
func (a *App) Unit() string {
	return a.Kind + "@" + a.Name + ".service"
}

var nameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// Load reads a manifest, applies defaults and validates it.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	m.Dir = filepath.Dir(abs)
	m.applyDefaults()
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &m, nil
}

func (m *Manifest) applyDefaults() {
	for sName, s := range m.Sessions {
		for _, a := range s.Apps {
			if a.Name == "" {
				// Convention: a single instance of a kind per session can
				// skip its name. Two → an explicit name is required (otherwise
				// the defaulted names collide and Validate rejects the manifest).
				a.Name = sName + "-" + a.Kind
			}
			if a.Kind == KindSooperLooper {
				if a.OSCPort == 0 {
					a.OSCPort = 9951
				}
				if a.Loops == 0 {
					a.Loops = 1
				}
				if a.Channels == 0 {
					a.Channels = 2
				}
				if a.LoopTime == 0 {
					a.LoopTime = 40
				}
			}
		}
	}
}

// Validate checks the manifest's overall consistency.
func (m *Manifest) Validate() error {
	if len(m.Sessions) == 0 {
		return fmt.Errorf("no sessions defined")
	}
	seen := map[string]string{} // name -> session
	ports := map[int]string{}   // OSC port -> instance
	for sName, s := range m.Sessions {
		if !nameRe.MatchString(sName) {
			return fmt.Errorf("session %q: invalid name (allowed: [a-zA-Z0-9_-])", sName)
		}
		if len(s.Apps) == 0 {
			return fmt.Errorf("session %q: no apps", sName)
		}
		for _, a := range s.Apps {
			if !nameRe.MatchString(a.Name) {
				return fmt.Errorf("session %q: app %q: invalid name (allowed: [a-zA-Z0-9_-])", sName, a.Name)
			}
			if other, dup := seen[a.Name]; dup {
				return fmt.Errorf("instance %q defined in sessions %q and %q: instance names must be unique across the whole manifest", a.Name, other, sName)
			}
			seen[a.Name] = sName

			switch a.Kind {
			case KindCarla:
				if a.Preset == "" {
					return fmt.Errorf("session %q: carla %q: 'preset' field required", sName, a.Name)
				}
				if a.OSCTCPPort != 0 {
					if err := claimPort(ports, a.OSCTCPPort, a.Name); err != nil {
						return err
					}
				}
			case KindHydrogen:
				if a.Song == "" {
					return fmt.Errorf("session %q: hydrogen %q: 'song' field required", sName, a.Name)
				}
			case KindSooperLooper:
				if err := claimPort(ports, a.OSCPort, a.Name); err != nil {
					return err
				}
			default:
				return fmt.Errorf("session %q: app %q: unknown kind %q (supported: carla, hydrogen, sooperlooper)", sName, a.Name, a.Kind)
			}
		}
		if s.Patchbay != nil && s.Patchbay.File == "" {
			return fmt.Errorf("session %q: patchbay missing 'file' field", sName)
		}
	}
	return nil
}

func claimPort(ports map[int]string, port int, instance string) error {
	if other, dup := ports[port]; dup {
		return fmt.Errorf("OSC port %d used by both %q and %q", port, other, instance)
	}
	ports[port] = instance
	return nil
}

// Resolve makes a manifest path absolute (relative to the manifest's directory).
func (m *Manifest) Resolve(p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(m.Dir, p)
}

// SessionNames returns the sorted session names.
func (m *Manifest) SessionNames() []string {
	names := make([]string, 0, len(m.Sessions))
	for n := range m.Sessions {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// AppNames returns the sorted instance names across all sessions.
func (m *Manifest) AppNames() []string {
	var names []string
	for _, s := range m.Sessions {
		for _, a := range s.Apps {
			names = append(names, a.Name)
		}
	}
	sort.Strings(names)
	return names
}

// FindApp looks up an instance by name across all sessions.
func (m *Manifest) FindApp(name string) (*App, bool) {
	for _, sName := range m.SessionNames() {
		for _, a := range m.Sessions[sName].Apps {
			if a.Name == name {
				return a, true
			}
		}
	}
	return nil, false
}

// ResolveApp resolves a command argument targeting an instance. It accepts an
// exact name or, failing that, a kind: if exactly one instance of that kind
// exists, there's no need to spell out the full <session>-<kind> name (e.g.
// "carla" → the single carla instance). The argument is resolved against the
// real kind, not the defaulted name, so the shortcut works even when the
// instance has an explicit name.
func (m *Manifest) ResolveApp(arg string) (*App, error) {
	if a, ok := m.FindApp(arg); ok {
		return a, nil
	}
	var byKind []*App
	for _, sName := range m.SessionNames() {
		for _, a := range m.Sessions[sName].Apps {
			if a.Kind == arg {
				byKind = append(byKind, a)
			}
		}
	}
	switch len(byKind) {
	case 1:
		return byKind[0], nil
	case 0:
		if _, isSession := m.Sessions[arg]; isSession {
			return nil, fmt.Errorf("%q is a session, not an instance — stomp ui targets an app (instances: %v)", arg, m.AppNames())
		}
		return nil, fmt.Errorf("instance %q not found (instances: %v)", arg, m.AppNames())
	default:
		names := make([]string, len(byKind))
		for i, a := range byKind {
			names[i] = a.Name
		}
		sort.Strings(names)
		return nil, fmt.Errorf("multiple instances of kind %q: specify the name (%v)", arg, names)
	}
}
