# Stompbox 🎛️

**Audio as Code** for a personal studio: declarative audio sessions,
versioned in Git, orchestrated by systemd (user mode) and wired by
qpwgraph under PipeWire/Wayland. No graphical session manager.

A *stompbox* is an effects pedal: you stomp on it to launch the session.

```console
$ stomp on bossa      # start the session
$ stomp off           # turn everything off
$ stomp status        # what's running?
```

## Principle

The source of truth is the [`stompbox.yaml`](stompbox.yaml) manifest.
systemd units are **generated artifacts**, never edited by hand.
Clone the repo + `stomp apply` = studio rebuilt.

```
stompbox.yaml ──(stomp apply)──▶ ~/.config/systemd/user/*.service, *.target
                                 ~/.config/stompbox/<kind>/<instance>.env
```

- A session is a systemd `.target` (`bossa.target`) that pulls in its apps
  via `Wants=` — a crashing app doesn't bring down the session.
- `StopWhenUnneeded=yes` in the templates: stopping the target actually
  stops all the instances.
- Routing is handled by `qpwgraph -m -a [-x] <file.qpwgraph>` with the
  patchbay **enabled**: connections are reapplied as soon as nodes appear,
  so startup order no longer matters.
- No `[Install]` section: nothing starts at boot, everything is imperative.

## Installation

```console
$ go build -o stomp ./cmd/stomp
$ ./stomp apply          # generate and install the units
$ ./stomp on bossa       # 🎸
```

## Commands

| Command | Effect |
|----------|-------|
| `stomp render [-o dir]` | Generate the artifacts into a directory (inspection) |
| `stomp apply` | Generate, install into `~/.config`, `daemon-reload` |
| `stomp on <session>` | Import the Wayland env then start `<session>.target` |
| `stomp off [session]` | Stop a session, or all if omitted |
| `stomp status` | State of the target and units of each session |
| `stomp ui <instance\|kind>` | Open the throwaway UI (carla-control / slgui attached to the engine). A kind is enough if it has a single instance |

All accept `-f <manifest>` (default: `stompbox.yaml`) and `-v` (verbose / debug logging).

## Manifest

```yaml
sessions:
  bossa:
    apps:
      - kind: carla            # headless host (--no-gui), UI via `stomp ui`
        name: clean            # optional — default: <session>-<kind> (bossa-carla)
        preset: presets/clean.carla
        osc_tcp_port: 1455     # OSC TCP control (CARLA_OSC_TCP_PORT env)
      - kind: hydrogen         # hydrogen -n --driver jack -s <song>
        name: bossa_groove
        song: songs/bossa.h2song
      - kind: sooperlooper     # headless engine, UI via `stomp ui`
        name: looper
        osc_port: 9951         # OSC over UDP
        loops: 2
        channels: 2
        looptime: 60           # seconds of memory per channel
        session: loops/bossa.slsess          # optional
        midi_binding: midi/footswitch.slb    # optional
    patchbay:
      file: patchbays/bossa.qpwgraph
      exclusive: true          # -x: disconnect anything not in the file
```

The `name` field is optional: by default an instance is named
`<session>-<kind>` (e.g. `bossa-carla`). So two apps of the same kind in a
session require an explicit `name`, otherwise their defaulted names collide.

Constraints validated on load: names `[a-zA-Z0-9_-]`, instance names unique
across the whole manifest, unique OSC ports, fields required per kind.

Presets, songs and patchbays live in the repo next to the manifest (relative
paths resolved against `stompbox.yaml`). To create a `.qpwgraph`: wire it up
once by hand in qpwgraph, save the patchbay, commit it.

## Development

```console
$ go test ./...
$ go test ./internal/render -update   # regenerate the goldens
```

See [PLAN.md](PLAN.md) for the full architecture and phasing
(phase 2: embedded web server `stomp serve`, phase 3: D-Bus, SSE,
OSC transport control).
