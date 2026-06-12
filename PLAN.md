# Stompbox — Development Plan

> **Audio as Code** for a personal studio: declarative audio sessions,
> versioned in Git, orchestrated by systemd (user mode) and wired by
> qpwgraph under PipeWire/Wayland.

A *stompbox* is an effects pedal: you stomp on it to launch the session.

```
stomp on bossa      # start the session
stomp off           # turn everything off
stomp status        # what's running?
```

---

## 1. Principle

The source of truth is a **declarative YAML manifest** (`stompbox.yaml`)
versioned in Git. systemd units are **generated artifacts**, never edited
by hand. Clone the repo + `stomp apply` = studio rebuilt.

No graphical session manager (NSM/RaySession): application lifecycle is
handled by systemd user, audio/MIDI routing by qpwgraph in enabled-patchbay
mode.

## 2. Architecture

A **single Go binary**, zero runtime dependency other than
systemd/PipeWire:

```
stomp render   → generates the systemd units from the YAML manifest
stomp apply    → render + copy into ~/.config/systemd/user + daemon-reload
stomp on/off   → start/stop session targets
stomp status   → state of the sessions
stomp ui       → launches an app's UI (e.g. slgui attached to the engine)
stomp serve    → web server (UI embedded via embed.FS)
```

### Example manifest

```yaml
# stompbox.yaml
sessions:
  bossa:
    apps:
      - kind: carla
        name: clean
        preset: presets/clean.carla
        osc_tcp_port: 1455      # explicit — no arithmetic on %i
      - kind: hydrogen
        name: bossa_groove
        song: songs/bossa.h2song
      - kind: sooperlooper
        name: looper
        osc_port: 9951          # note: UDP (Carla, on the other hand, can do TCP)
        loops: 2
        channels: 2
        looptime: 60            # seconds of memory per channel
        session: loops/bossa.slsess        # optional
        midi_binding: midi/footswitch.slb  # optional
    patchbay:
      file: patchbays/bossa.qpwgraph
      exclusive: true           # qpwgraph -x: the file's wiring is authoritative
```

### Repo layout

```
stompbox/
├── cmd/stomp/main.go
├── internal/
│   ├── manifest/        # YAML parsing + validation
│   ├── render/          # text/template → units + env files
│   ├── systemd/         # systemctl --user wrapper (proto), D-Bus later
│   └── server/          # HTTP API + embedded UI
├── web/                 # static UI (embedded in the binary)
├── templates/           # .service / .target templates
├── examples/bossa/      # complete example session
├── stompbox.yaml
├── PLAN.md
└── README.md
```

## 3. systemd generation rules (validated fixes)

CLI options verified on the target machine (2026-06):

| App | CLI reality |
|-----|-------------|
| qpwgraph | No `-p` flag: the patchbay file is **positional**. `qpwgraph -m -a [-x] bossa.qpwgraph` |
| Carla | No `carla-headless` binary: `carla --no-gui`. OSC port via `CARLA_OSC_TCP_PORT` / `CARLA_OSC_UDP_PORT` env. Real projects are in **`.carxp`** format (not `.carla`) — those from the old RaySession session are saved by `carla-patchbay` |
| Element | No headless/no-gui mode at all: `element <session.els>` always opens its window, which **is** the UI — graphical unit like qpwgraph/Hydrogen, no `stomp ui`. Session files are **`.els`** (JUCE ValueTree XML) |
| Hydrogen | `-s file.h2song`, add `-n` (no splash) and `--driver jack` |
| SooperLooper | Native headless engine: `sooperlooper -q -j sl_<name> -p <port> -l <loops> -c <ch> -t <sec> [-L session] [-m binding]`. Separate UI: `slgui` |

Rules applied in the templates:

- **`StopWhenUnneeded=yes`** in every service template: `Requires=` doesn't
  propagate stop; with this flag, `stomp off` (stopping the target) makes
  the instances "unneeded" and systemd stops them. This is THE trap of a
  naive design.
- **`Wants=`** rather than `Requires=` in the targets: an app that crashes
  on startup doesn't fail the whole session.
- **`BindsTo=graphical-session.target`** + `After=graphical-session.target`
  on graphical units: clean death on logout.
- **No `[Install]` section**: purely imperative startup via
  `systemctl --user start <session>.target`.
- **One env file per instance** (`EnvironmentFile=%h/.config/stompbox/<kind>/%i.env`)
  carrying the OSC port, preset/song/session path — cleanly solves the
  "one unique port per instance" requirement.
- **Deterministic JACK client name per instance** (e.g. `-j sl_%i` for
  SooperLooper, `--cnprefix` for multi-client Carla): essential for the
  qpwgraph patchbay to apply reliably and for multiple instances.
- The server/CLI runs `systemctl --user import-environment WAYLAND_DISPLAY DISPLAY`
  as a safety net before any start (GNOME/KDE already do this at login;
  `XDG_RUNTIME_DIR` is known to systemd user by construction).

### Why an enabled-patchbay qpwgraph is the key strength

**Enabled** patchbay (`-a`): qpwgraph reapplies connections on the fly as
soon as nodes appear. This eliminates the classic race ("the app isn't
ready when we wire it up") — the startup order of units becomes almost
irrelevant.

### App UIs: not in systemd

`slgui` (and the Carla GUIs) are throwaway clients: open them to tweak
settings, close them, the engine keeps running. Exposed via `stomp ui <instance>`.

## 4. Phasing

### Phase 0 — validate the audio, zero lines of Go (one evening)

The real risk isn't the code, it's the behavior of qpwgraph/Carla/systemd.
Write the bossa session's units **by hand** in `~/.config/systemd/user/`
and check:

- [ ] Does `qpwgraph -m -a` survive without a system tray under the target
      Wayland compositor? (GNOME Wayland has no native tray; KDE does)
- [ ] Is `CARLA_OSC_TCP_PORT` honored by the installed Carla?
- [ ] Does `carla --no-gui presets/jazz.carxp` correctly load a project
      saved by `carla-patchbay` (migrated RaySession sessions)?
- [ ] Does `StopWhenUnneeded=yes` actually stop everything on
      `stop <session>.target`?
- [ ] Does `slgui` attach/detach from the engine without interrupting the
      loops?
- [ ] Create `patchbays/homestudio.qpwgraph`: wire it up once in qpwgraph
      following `patchbays/homestudio.raypatch.xml`, save, commit.

The validated units become phase 1's templates.

### Phase 1 — `stomp` CLI (the core)

Manifest → generation → `apply` / `on` / `off` / `status` / `ui`.
The systemd wrapper just does `exec systemctl --user ...`: trivial to
debug. Golden tests on unit rendering (YAML → text).
**By the end of this phase the project is usable day-to-day without a UI.**

### Phase 2 — `stomp serve`, the web UI

- Minimal REST API: `GET /api/sessions`,
  `POST /api/sessions/{name}/start|stop`, `GET /api/status`
- htmx or vanilla JS page, big pedalboard-style buttons
- No Node toolchain: everything in `embed.FS`, stdlib `net/http`
- Wayland `import-environment` on server startup
- Target use case: a tablet sitting on the amp

### Phase 1.5 — RaySession migration (backlog)

- `stomp import-raypatch <patch.xml>`: convert ray-jackpatch connections
  (e.g. `patchbays/homestudio.raypatch.xml`) into a `.qpwgraph` file, to
  migrate old sessions without rewiring by hand.
- Future kinds identified in existing RaySession sessions:
  `ardour`, `midi2osc`.
- SooperLooper state (`.slsess` + loop wavs) stays out of Git: it's
  playing state, not configuration. Only the MIDI binding
  (`.slb`, Morningstar MC8 pedalboard) is versioned.

### Phase 3 — refinement

- Replace `systemctl` execs with D-Bus
  (`github.com/coreos/go-systemd/v22/dbus`): real-time state
- Push state changes to the UI via SSE
- Per-session journald logs in the UI
- Transport control: **per-loop record/overdub/undo** buttons via
  SooperLooper OSC (`/sl/0/hit record`, `/sl/-1/hit trigger`…), Hydrogen
  play/stop
- USB MIDI pedalboard wired directly into the SooperLooper engine (`-m`)

## 5. Accepted technical choices

- **No web framework or frontend build**: stdlib + embedded htmx, a
  single binary that can be copied anywhere.
- **systemctl via exec first, D-Bus later**: the prototype stays
  readable, the `internal/systemd` interface isolates the change.
- **Presets/songs/patchbays/sessions live in the Git repo** next to the
  manifest.

## 6. Known risks

- No system tray under GNOME Wayland → `qpwgraph -m` behavior to validate
  in phase 0 (otherwise: leave it as a normal window on another workspace,
  or an AppIndicator extension).
- Hydrogen installed as 1.2.0-**beta** (2024) → needs updating.
- SooperLooper OSC over **UDP** vs Carla over TCP: `stomp ui` and the web
  controls will need to handle both.
- Tkinter dropped in favor of the web — no more XWayland question.
