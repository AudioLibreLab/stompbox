# Stompbox 🎛️

**Audio as Code** pour studio personnel : sessions audio déclaratives,
versionnées dans Git, orchestrées par systemd (mode utilisateur) et câblées
par qpwgraph sous PipeWire/Wayland. Pas de gestionnaire de session graphique.

Une *stompbox*, c'est une pédale d'effet : on écrase la pédale pour lancer
la session.

```console
$ stomp on bossa      # démarre la session
$ stomp off           # coupe tout
$ stomp status        # qu'est-ce qui tourne ?
```

## Principe

La source de vérité est le manifeste [`stompbox.yaml`](stompbox.yaml).
Les units systemd sont des **artefacts générés**, jamais édités à la main.
Cloner le dépôt + `stomp apply` = studio reconstruit.

```
stompbox.yaml ──(stomp apply)──▶ ~/.config/systemd/user/*.service, *.target
                                 ~/.config/stompbox/<kind>/<instance>.env
```

- Une session = un `.target` systemd (`bossa.target`) qui tire ses apps
  via `Wants=` — une app qui plante ne fait pas tomber la session.
- `StopWhenUnneeded=yes` dans les gabarits : arrêter le target arrête
  réellement toutes les instances.
- Le routage est confié à `qpwgraph -m -a [-x] <fichier.qpwgraph>` en
  patchbay **activé** : les connexions sont réappliquées dès que les nœuds
  apparaissent, l'ordre de démarrage devient indifférent.
- Pas de section `[Install]` : rien ne démarre au boot, tout est impératif.

## Installation

```console
$ go build -o stomp ./cmd/stomp
$ ./stomp apply          # génère et installe les units
$ ./stomp on bossa       # 🎸
```

## Commandes

| Commande | Effet |
|----------|-------|
| `stomp render [-o dir]` | Génère les artefacts dans un répertoire (inspection) |
| `stomp apply` | Génère, installe dans `~/.config`, `daemon-reload` |
| `stomp on <session>` | Importe l'env Wayland puis démarre `<session>.target` |
| `stomp off [session]` | Arrête une session, ou toutes si omis |
| `stomp status` | État du target et des units de chaque session |
| `stomp ui <instance\|kind>` | Ouvre l'IHM jetable (carla-control / slgui attaché au moteur). Un kind suffit s'il n'a qu'une instance |

Toutes acceptent `-f <manifeste>` (défaut : `stompbox.yaml`).

## Manifeste

```yaml
sessions:
  bossa:
    apps:
      - kind: carla            # host headless (--no-gui), IHM via `stomp ui`
        name: clean            # optionnel — défaut: <session>-<kind> (bossa-carla)
        preset: presets/clean.carla
        osc_tcp_port: 1455     # contrôle OSC TCP (env CARLA_OSC_TCP_PORT)
      - kind: hydrogen         # hydrogen -n --driver jack -s <song>
        name: bossa_groove
        song: songs/bossa.h2song
      - kind: sooperlooper     # moteur headless, IHM via `stomp ui`
        name: looper
        osc_port: 9951         # OSC en UDP
        loops: 2
        channels: 2
        looptime: 60           # secondes de mémoire par canal
        session: loops/bossa.slsess          # optionnel
        midi_binding: midi/footswitch.slb    # optionnel
    patchbay:
      file: patchbays/bossa.qpwgraph
      exclusive: true          # -x : déconnecte ce qui n'est pas dans le fichier
```

Le champ `name` est optionnel : par défaut une instance s'appelle
`<session>-<kind>` (ex. `bossa-carla`). Du coup deux apps du même kind dans
une session imposent un `name` explicite, sinon leurs noms défautés
collisionnent.

Contraintes validées au chargement : noms `[a-zA-Z0-9_-]`, noms d'instance
uniques sur tout le manifeste, ports OSC uniques, champs requis par kind.

Les presets, songs et patchbays vivent dans le dépôt à côté du manifeste
(chemins relatifs résolus par rapport à `stompbox.yaml`). Pour créer un
`.qpwgraph` : câbler une fois à la main dans qpwgraph, sauvegarder le
patchbay, commiter.

## Développement

```console
$ go test ./...
$ go test ./internal/render -update   # régénère les goldens
```

Voir [PLAN.md](PLAN.md) pour l'architecture complète et le phasage
(phase 2 : serveur web embarqué `stomp serve`, phase 3 : D-Bus, SSE,
contrôle transport OSC).
