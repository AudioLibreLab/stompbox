# Stompbox — Plan de développement

> **Audio as Code** pour studio personnel : sessions audio déclaratives,
> versionnées dans Git, orchestrées par systemd (mode utilisateur) et
> câblées par qpwgraph sous PipeWire/Wayland.

Une *stompbox*, c'est une pédale d'effet : on écrase la pédale pour lancer
la session.

```
stomp on bossa      # démarre la session
stomp off           # coupe tout
stomp status        # qu'est-ce qui tourne ?
```

---

## 1. Principe

La source de vérité est un **manifeste YAML déclaratif** (`stompbox.yaml`)
versionné dans Git. Les units systemd sont des **artefacts générés**, jamais
édités à la main. Cloner le dépôt + `stomp apply` = studio reconstruit.

Pas de gestionnaire de session graphique (NSM/RaySession) : le cycle de vie
des applications est confié à systemd user, le routage audio/MIDI à qpwgraph
en mode patchbay activé.

## 2. Architecture

Un **seul binaire Go**, zéro dépendance d'exécution autre que
systemd/PipeWire :

```
stomp render   → génère les units systemd depuis le manifeste YAML
stomp apply    → render + copie dans ~/.config/systemd/user + daemon-reload
stomp on/off   → start/stop des targets de session
stomp status   → état des sessions
stomp gui      → lance l'IHM d'une app (ex: slgui attaché au moteur)
stomp serve    → serveur web (UI embarquée via embed.FS)
```

### Manifeste exemple

```yaml
# stompbox.yaml
sessions:
  bossa:
    apps:
      - kind: carla
        name: clean
        preset: presets/clean.carla
        osc_tcp_port: 1455      # explicite — pas d'arithmétique sur %i
        gui: false
      - kind: hydrogen
        name: bossa_groove
        song: songs/bossa.h2song
      - kind: sooperlooper
        name: looper
        osc_port: 9951          # attention : UDP (Carla, lui, peut faire TCP)
        loops: 2
        channels: 2
        looptime: 60            # secondes de mémoire par canal
        session: loops/bossa.slsess        # optionnel
        midi_binding: midi/footswitch.slb  # optionnel
    patchbay:
      file: patchbays/bossa.qpwgraph
      exclusive: true           # qpwgraph -x : le câblage du fichier fait loi
```

### Arborescence du dépôt

```
stompbox/
├── cmd/stomp/main.go
├── internal/
│   ├── manifest/        # parsing + validation YAML
│   ├── render/          # text/template → units + fichiers d'env
│   ├── systemd/         # wrapper systemctl --user (proto), D-Bus ensuite
│   └── server/          # API HTTP + UI embarquée
├── web/                 # UI statique (embarquée dans le binaire)
├── templates/           # gabarits .service / .target
├── examples/bossa/      # session d'exemple complète
├── stompbox.yaml
├── PLAN.md
└── README.md
```

## 3. Règles de génération systemd (corrections validées)

Options CLI vérifiées sur la machine cible (2026-06) :

| App | Réalité CLI |
|-----|-------------|
| qpwgraph | Pas de flag `-p` : le fichier patchbay est **positionnel**. `qpwgraph -m -a [-x] bossa.qpwgraph` |
| Carla | Pas de binaire `carla-headless` : `carla --no-gui`. Port OSC via env `CARLA_OSC_TCP_PORT` / `CARLA_OSC_UDP_PORT` |
| Hydrogen | `-s fichier.h2song`, ajouter `-n` (pas de splash) et `--driver jack` |
| SooperLooper | Moteur headless natif : `sooperlooper -q -j sl_<nom> -p <port> -l <loops> -c <ch> -t <sec> [-L session] [-m binding]`. IHM séparée : `slgui` |

Règles appliquées dans les gabarits :

- **`StopWhenUnneeded=yes`** dans chaque template de service : `Requires=` ne
  propage pas l'arrêt ; avec ce flag, `stomp off` (stop du target) rend les
  instances « inutiles » et systemd les arrête. C'est LE piège du design naïf.
- **`Wants=`** plutôt que `Requires=` dans les targets : une app qui plante au
  lancement ne fait pas échouer toute la session.
- **`BindsTo=graphical-session.target`** + `After=graphical-session.target`
  sur les unités graphiques : mort propre au logout.
- **Pas de section `[Install]`** : démarrage purement impératif via
  `systemctl --user start <session>.target`.
- **Un fichier d'env par instance** (`EnvironmentFile=%h/.config/stompbox/<kind>/%i.env`)
  portant port OSC, chemin de preset/song/session — résout proprement le
  besoin « un port unique par instance ».
- **Nom de client JACK déterministe par instance** (ex: `-j sl_%i` pour
  SooperLooper, `--cnprefix` pour Carla multi-client) : indispensable pour que
  le patchbay qpwgraph s'applique de façon fiable et pour les instances
  multiples.
- Le serveur/CLI fait `systemctl --user import-environment WAYLAND_DISPLAY DISPLAY`
  par sécurité avant tout start (GNOME/KDE le font déjà au login ;
  `XDG_RUNTIME_DIR` est connu de systemd user par construction).

### Pourquoi qpwgraph en patchbay activé est le point fort

Patchbay **activé** (`-a`) : qpwgraph réapplique les connexions au fil de
l'eau dès que les nœuds apparaissent. Ça élimine la course classique
(« l'app n'est pas prête quand on câble ») — l'ordre de démarrage des unités
devient presque indifférent.

### IHM des apps : pas dans systemd

`slgui` (et les GUIs Carla) sont des clients jetables : on les ouvre pour
régler, on les ferme, le moteur continue. Exposé via `stomp gui <instance>`.

## 4. Phasage

### Phase 0 — valider l'audio, zéro ligne de Go (une soirée)

Le vrai risque n'est pas le code, c'est le comportement de
qpwgraph/Carla/systemd. Écrire **à la main** les units de la session bossa
dans `~/.config/systemd/user/` et vérifier :

- [ ] `qpwgraph -m -a` survit-il sans tray système sous le compositeur
      Wayland cible ? (GNOME Wayland n'a pas de tray natif ; KDE oui)
- [ ] `CARLA_OSC_TCP_PORT` est-il honoré par le Carla installé ?
- [ ] `StopWhenUnneeded=yes` arrête-t-il bien tout au `stop bossa.target` ?
- [ ] `slgui` s'attache/se détache du moteur sans interrompre les boucles ?

Les units validés deviennent les gabarits de la phase 1.

### Phase 1 — CLI `stomp` (le cœur)

Manifeste → génération → `apply` / `on` / `off` / `status` / `gui`.
Le wrapper systemd fait juste `exec systemctl --user ...` : trivial à
déboguer. Tests golden sur le rendu des units (YAML → texte).
**À la fin de cette phase le projet est utilisable au quotidien sans UI.**

### Phase 2 — `stomp serve`, l'IHM web

- API REST minimale : `GET /api/sessions`,
  `POST /api/sessions/{name}/start|stop`, `GET /api/status`
- Page htmx ou vanilla JS, gros boutons façon pédalier
- Pas de toolchain Node : tout en `embed.FS`, stdlib `net/http`
- `import-environment` Wayland au démarrage du serveur
- Cas d'usage cible : tablette posée sur l'ampli

### Phase 3 — affinage

- Remplacer les exec `systemctl` par D-Bus
  (`github.com/coreos/go-systemd/v22/dbus`) : états temps réel
- Push des changements d'état vers l'UI en SSE
- Logs journald par session dans l'UI
- Contrôle transport : boutons **record/overdub/undo par boucle** via l'OSC
  de SooperLooper (`/sl/0/hit record`, `/sl/-1/hit trigger`…), play/stop
  Hydrogen
- Pédalier MIDI USB branché directement sur le moteur SooperLooper (`-m`)

## 5. Choix techniques assumés

- **Pas de framework web ni de build front** : stdlib + htmx embarqué,
  un binaire unique copiable n'importe où.
- **systemctl en exec d'abord, D-Bus ensuite** : le proto reste lisible,
  l'interface `internal/systemd` isole le changement.
- **Presets/songs/patchbays/sessions vivent dans le dépôt Git** à côté du
  manifeste.

## 6. Risques connus

- Tray système absent sous GNOME Wayland → comportement de `qpwgraph -m` à
  valider en phase 0 (sinon : le laisser en fenêtre normale sur un autre
  workspace, ou extension AppIndicator).
- Hydrogen installé en 1.2.0-**beta** (2024) → à mettre à jour.
- OSC SooperLooper en **UDP** vs Carla en TCP : `stomp gui` et les contrôles
  web devront gérer les deux.
- Tkinter abandonné au profit du web — plus de question XWayland.
