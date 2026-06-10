// Package manifest charge et valide le manifeste déclaratif stompbox.yaml,
// la source de vérité des sessions du studio.
package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"gopkg.in/yaml.v3"
)

// Kinds supportés par le générateur.
const (
	KindCarla        = "carla"
	KindHydrogen     = "hydrogen"
	KindSooperLooper = "sooperlooper"
)

// Manifest est la racine du fichier stompbox.yaml.
type Manifest struct {
	Sessions map[string]*Session `yaml:"sessions"`

	// Dir est le répertoire absolu contenant le manifeste ;
	// les chemins relatifs du manifeste sont résolus par rapport à lui.
	Dir string `yaml:"-"`
}

// Session décrit un ensemble d'applications et leur câblage.
type Session struct {
	Apps     []*App    `yaml:"apps"`
	Patchbay *Patchbay `yaml:"patchbay"`
}

// App décrit une instance d'application. Les champs sont spécifiques au kind ;
// la validation impose ceux qui sont requis.
type App struct {
	Kind string `yaml:"kind"`
	Name string `yaml:"name"`

	// carla
	Preset     string `yaml:"preset"`
	OSCTCPPort int    `yaml:"osc_tcp_port"`
	GUI        bool   `yaml:"gui"`

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

// Patchbay décrit le fichier de brassage qpwgraph d'une session.
type Patchbay struct {
	File      string `yaml:"file"`
	Exclusive bool   `yaml:"exclusive"`
}

// Unit retourne le nom de l'unité systemd de l'instance (ex: carla@clean.service).
func (a *App) Unit() string {
	return a.Kind + "@" + a.Name + ".service"
}

var nameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// Load lit, applique les défauts et valide un manifeste.
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
	for _, s := range m.Sessions {
		for _, a := range s.Apps {
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

// Validate vérifie la cohérence globale du manifeste.
func (m *Manifest) Validate() error {
	if len(m.Sessions) == 0 {
		return fmt.Errorf("aucune session définie")
	}
	seen := map[string]string{}   // "kind@name" -> session
	ports := map[int]string{}     // port OSC -> instance
	for sName, s := range m.Sessions {
		if !nameRe.MatchString(sName) {
			return fmt.Errorf("session %q: nom invalide (autorisé: [a-zA-Z0-9_-])", sName)
		}
		if len(s.Apps) == 0 {
			return fmt.Errorf("session %q: aucune app", sName)
		}
		for _, a := range s.Apps {
			if !nameRe.MatchString(a.Name) {
				return fmt.Errorf("session %q: app %q: nom invalide (autorisé: [a-zA-Z0-9_-])", sName, a.Name)
			}
			key := a.Kind + "@" + a.Name
			if other, dup := seen[key]; dup {
				return fmt.Errorf("instance %q définie dans les sessions %q et %q: les noms d'instance doivent être uniques par kind sur tout le manifeste", key, other, sName)
			}
			seen[key] = sName

			switch a.Kind {
			case KindCarla:
				if a.Preset == "" {
					return fmt.Errorf("session %q: carla %q: champ 'preset' requis", sName, a.Name)
				}
				if a.OSCTCPPort != 0 {
					if err := claimPort(ports, a.OSCTCPPort, key); err != nil {
						return err
					}
				}
			case KindHydrogen:
				if a.Song == "" {
					return fmt.Errorf("session %q: hydrogen %q: champ 'song' requis", sName, a.Name)
				}
			case KindSooperLooper:
				if err := claimPort(ports, a.OSCPort, key); err != nil {
					return err
				}
			default:
				return fmt.Errorf("session %q: app %q: kind %q inconnu (supportés: carla, hydrogen, sooperlooper)", sName, a.Name, a.Kind)
			}
		}
		if s.Patchbay != nil && s.Patchbay.File == "" {
			return fmt.Errorf("session %q: patchbay sans champ 'file'", sName)
		}
	}
	return nil
}

func claimPort(ports map[int]string, port int, instance string) error {
	if other, dup := ports[port]; dup {
		return fmt.Errorf("port OSC %d utilisé par %q et %q", port, other, instance)
	}
	ports[port] = instance
	return nil
}

// Resolve rend absolu un chemin du manifeste (relatif au répertoire du manifeste).
func (m *Manifest) Resolve(p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(m.Dir, p)
}

// SessionNames retourne les noms de session triés.
func (m *Manifest) SessionNames() []string {
	names := make([]string, 0, len(m.Sessions))
	for n := range m.Sessions {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// FindApp cherche une instance par nom dans toutes les sessions.
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
