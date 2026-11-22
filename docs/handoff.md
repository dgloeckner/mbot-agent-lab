# Handoff: mBot2 Agent Lab

Nov 22, 2026 · @Daniel

## Zweck und Kontext

Dieses Dokument übergibt das Projekt „mBot2 Agent Lab“ an Claude Code: ein Lab, in dem Teams von 15-Jährigen KI-Agenten anleiten, die echte Roboter steuern. Es enthält alle bisher getroffenen Entscheidungen, die Spezifikation des gesamten Stacks, die Deployment-Optionen und das Lab-Konzept.

- **Hardware:** 5 Makeblock mBot2 mit CyberPi (ESP32, MicroPython), Ultraschallsensor, Quad-RGB-Sensor, Encoder-Motoren, Display, Lautsprecher und LEDs.
- **Nutzer:** Teams an Windows-PCs im Firmennetz. Sie brauchen nur den Browser, kein Konto bei einem KI-Anbieter und keine lokale Software.
- **Kernidee:** Jedes Team schreibt und deployt den Prompt seines Agenten. Der Agent läuft als Schleife im Server, steuert den Roboter des Teams über Tools und reagiert auf Nachrichten vom Roboter, vom Team und von anderen Agenten.
- **Lernziele:** präzise anleiten, Fehler und Grenzen von Agenten erkennen, Tools entwerfen, Angriffe wie Prompt-Injection verstehen, Token als Kostenfaktor begreifen.

**Ziele für die Software**

- Ein Go-Binary als Server mit Broker, Agenten und Weboberfläche, dazu ein MicroPython-Skript pro Roboter.
- Läuft lokal zum Entwickeln und auf einer Azure-VM für das Lab.
- Ist ohne Hardware entwickelbar und testbar, über einen Roboter-Simulator.

**Vorerst nicht im Umfang**

- Kamera, Kartierung und Bahnplanung.
- Sprach-Ein- und -Ausgabe. Die Cloud-Spracherkennung von Makeblock wird bewusst nicht genutzt.
- Benutzerverwaltung über Team-Links und ein Admin-Passwort hinaus.

## Entscheidungen im Überblick

Diese Punkte sind entschieden und gelten als Vorgabe. Abweichungen bitte erst vorschlagen, nicht stillschweigend umsetzen.

| Thema | Entscheidung | Grund |
| --- | --- | --- |
| Repo-Name | `mbot-agent-lab`, ein Monorepo | alles an einem Ort |
| Server | Go, ein einzelnes Binary | Broker einbettbar, einfache Verteilung |
| Roboter-Code | MicroPython im Upload-Modus, Upload über mBlock | einzige Option auf dem CyberPi |
| Transport Roboter–Server | MQTT, QoS 0, kompaktes JSON | Last Will, Retained State, leicht zu beobachten |
| Broker | mochi-mqtt, im Server eingebettet | kein zweiter Prozess |
| Agenten | eine Schleife pro Team im Server, als Aktor mit Postfach | Ereignisse können den Agenten wecken |
| Nachrichtenquellen | Roboter, eigenes Team, andere Agenten, System | ein Umschlag, Vertrauen nach Quelle |
| Prompts | vom Team deploybar, versioniert | Vergleich und Rollback |
| Gedächtnis | Arbeitskontext, Notizen, Protokoll | Lernen über Läufe, Material für den Lügendetektor |
| Roboter-Zuordnung | ein Roboter gehört immer exklusiv einem Besitzer | keine Sperrlogik nötig |
| Sprachmodell | extern und günstig über eine OpenAI-kompatible API, Start mit Mistral Small, Ollama als Rückfall | läuft parallel für alle Teams, kein GPU-Rechner |
| UI | Webseiten aus dem Go-Server, nur Browser | keine Software auf den Lab-PCs |
| Live-Daten in der UI | Server-Sent Events | gehen zuverlässig durch Firmenproxys |
| Speicherung | SQLite mit reinem Go-Treiber | es bleibt bei einem Binary |
| Ziel-Deployment | Server auf Azure-VM, Roboter im Gäste-WLAN, PCs im Firmennetz | das Firmennetz bleibt unberührt |
| Reflexe | Nothalt, Limits und Watchdog laufen auf dem Roboter | das Modell braucht Sekunden pro Entscheidung |
| MCP | optionale zweite Tür auf dieselbe Tool-Registry | für externe Clients wie Open WebUI |
| Sprache | UI und Lab-Material auf Deutsch, Code und Kommentare auf Englisch | Zielgruppe und übliche Praxis |

## Repo-Struktur

Ein Monorepo mit vier Teilen: Roboter, Server, Simulator und Deployment. Die UI liegt im Server und wird in das Binary eingebettet.

```
mbot-agent-lab/
├── CLAUDE.md                  # Kurzfassung dieses Handoffs für Claude Code
├── README.md
├── Makefile                   # dev, test, sim, bundle, build, deploy
├── docs/
│   ├── handoff.md             # dieses Dokument als Markdown
│   ├── protocol.md            # MQTT-Topics und Nachrichten, Quelle der Wahrheit
│   ├── tools.md               # Tool-Katalog
│   └── lab/                   # Szenarien, Ablaufpläne, Prompt-Vorlagen für Teams
├── robot/
│   ├── src/                   # Module: net, mqtt, loop, skills, reflexes, events, ui
│   ├── tools/bundle.py        # fügt src/ zu einer einzelnen main.py zusammen
│   ├── config_example.py      # WLAN, Broker, Roboter-ID; echte config.py nie ins Repo
│   └── dist/main.py           # Ergebnis des Bundlings, Upload über mBlock
├── sim/                       # Python: führt das echte Roboterskript gegen Attrappen aus
│   ├── fakes/                 # cyberpi.py, mbot2.py, mbuild.py, umqtt/simple.py
│   ├── world.py               # 2D-Welt: Wände, Farbfelder, Linien, Roboterpose
│   ├── worlds/                # labyrinth.json, arena.json
│   └── run.py                 # startet n simulierte Roboter
├── server/
│   ├── cmd/labserver/main.go
│   ├── internal/
│   │   ├── config/            # YAML plus Umgebungsvariablen
│   │   ├── protocol/          # Nachrichtentypen für MQTT
│   │   ├── broker/            # eingebetteter MQTT-Broker, Auth, ACL
│   │   ├── robots/            # Registry, Zustand, Besitz, Heartbeat
│   │   ├── tools/             # Tool-Registry und Implementierungen
│   │   ├── agent/             # Aktor, Postfach, Schleife, Kontextaufbau
│   │   ├── llm/               # OpenAI-kompatibler Client, Anbieter, Kostenzählung
│   │   ├── memory/            # Notizen, Zusammenfassung des Verlaufs
│   │   ├── store/             # SQLite, Migrationen
│   │   ├── web/               # Handler, SSE, Templates, statische Dateien per embed
│   │   ├── auth/              # Team-Links, Admin, Roboter-Zugänge
│   │   └── mcp/               # optionaler MCP-Endpunkt
│   └── go.mod
├── configs/
│   ├── lab.example.yaml
│   └── profiles/              # einzel.yaml, duo.yaml, koordinator.yaml
└── deploy/
    ├── azure/                 # VM anlegen, cloud-init
    ├── systemd/labserver.service
    └── local/                 # Beispielkonfigurationen für die Entwicklung
```

**Warum der Simulator in Python ist:** Er führt die echte `main.py` unter normalem Python aus und ersetzt nur die Herstellermodule durch Attrappen. So wird der Robotercode selbst getestet, nicht ein Nachbau. Der Simulator spricht über einen echten MQTT-Client mit dem Server.

**Warum ein Bundle:** Ob mBlock mehrere Dateien auf den CyberPi laden kann, ist ungeklärt. Das Repo erzeugt deshalb immer eine einzelne `main.py`.

## Roboter: MicroPython auf dem CyberPi

Auf jedem Roboter läuft ein einzelnes Skript im Upload-Modus, das nach dem Einschalten startet, sich mit dem Broker verbindet und niemals blockiert.

**Rahmenbedingungen**

- Upload per USB über mBlock. Das Repo liefert dafür `robot/dist/main.py`.
- Konfiguration pro Roboter: `ROBOT_ID`, WLAN-Name und Passwort, Broker-Host, Port, Benutzer, Passwort, TLS an oder aus.
- Der CyberPi kann nur 2,4-GHz-WLAN.
- Module: `cyberpi`, `mbot2`, `mbuild` vom Hersteller, dazu ein MQTT-Client und JSON. Vermutlich ist `umqtt.simple` verfügbar, das ist zu prüfen.
- Der Speicher ist knapp: kurze Nachrichten, keine großen Puffer, keine Bibliotheken außer den genannten.

**Hauptschleife, Ziel 20 bis 50 ms pro Durchlauf**

1. Befehle abholen, nicht blockierend (`check_msg`).
2. Den aktiven Skill einen Schritt weiterführen. Jeder Skill ist ein Zustandsautomat.
3. Reflexe prüfen.
4. Ereignisse bei Zustandswechsel senden.
5. Zustand senden: sofort bei Änderung, sonst zwei- bis fünfmal pro Sekunde.
6. Heartbeat des Servers überwachen.

**Regel: nichts blockiert**

- Die bequemen Fahrbefehle für feste Strecke und festen Winkel kehren erst nach der Bewegung zurück. In der Zeit käme ein Stopp nicht an. Sie werden deshalb nicht verwendet.
- Stattdessen: Motoren mit Geschwindigkeit starten, Encoder-Winkel und Gyro-Yaw in der Schleife mitlesen und selbst anhalten.
- Töne nur über die nicht blockierende Wiedergabe.
- Threads sind eine mögliche Alternative, aber erst nach einem Test, ob ein Stopp aus einem zweiten Thread eine laufende Fahrt abbricht.

**Skills**

| Skill | Verhalten |
| --- | --- |
| `drive(cm, speed)` | fährt eine Strecke, Messung über Encoder |
| `turn(deg)` | dreht auf der Stelle, Messung über Gyro; positiv ist rechts |
| `drive_until(max_cm, stop_below_cm, stop_on_color)` | fährt, bis Abstand, Farbe oder Maximalstrecke erreicht ist |
| `follow_line_until(stop_on_color, max_cm)` | Linienregelung auf dem Roboter bis zur Abbruchbedingung |
| `retrace_path()` | fährt das Fahrtenlog rückwärts ab |
| `stop()` | hält sofort an, hat immer Vorrang, ist idempotent |
| `set_leds`, `show_text`, `play_sound`, `play_melody` | Ausgabe ohne Bewegung |
| `gesture(kind)` | `nod`, `shake`, `spin` als kurze Bewegungsmuster |
| `identify()` | blinkt und piept, damit das Team seinen Roboter findet |

**Reflexe, unabhängig vom Server**

- Nothalt, wenn bei Vorwärtsfahrt ein Hindernis näher als die Schwelle ist. Standard: 6 cm.
- Obergrenzen für Tempo, Strecke und Dauer pro Befehl.
- Watchdog: 3 Sekunden ohne Heartbeat vom Server führen zu Motorstopp und der Anzeige „getrennt“.
- Bei Verlust von WLAN oder Broker: stoppen, dann mit wachsender Pause neu verbinden.
- Umgekippt oder angehoben, erkannt über Pitch und Roll: stoppen und Ereignis senden.

**Ereignisse, mit Entprellung und Hysterese**

`obstacle_near`, `obstacle_clear`, `color_changed`, `tilted`, `button_a`, `button_b`, `clap`, `battery_low`, `skill_done`, `skill_aborted`

**Fahrtenlog**

- Liste der abgeschlossenen Segmente, je `drive` oder `turn` mit Wert.
- Grundlage für `retrace_path`. Wird per Befehl und beim Besitzerwechsel geleert.

**Anzeige am Roboter**

- Display: Roboter-ID, Besitzer, Verbindungsstatus, zuletzt gesagter Text.
- LEDs: Teamfarbe. Pulsierend, solange der Agent denkt. Den Zustand meldet der Server.

**Hersteller-APIs, die voraussichtlich gebraucht werden**

Die Namen stammen aus der Herstellerdoku und aus der Erinnerung. Vor der Verwendung gegen die Doku in den Referenzen prüfen.

- WLAN: `cyberpi.wifi.connect`, `cyberpi.wifi.is_connect`
- Fahren: `mbot2.drive_speed`, `mbot2.forward`, `mbot2.EM_stop`, Encoder-Winkel lesen
- Lage: `cyberpi.get_yaw`, `get_pitch`, `get_roll`, `reset_yaw`
- Abstand: `mbuild.ultrasonic2.get`
- Boden: `mbuild.quad_rgb_sensor.get_line_sta`, `get_offset_track`, Farberkennung pro Kanal
- Ausgabe: `cyberpi.led.on`, `cyberpi.console.println`, `cyberpi.display.show_label`, `cyberpi.audio.play`, `play_tone`, `set_vol`
- Sonstiges: `cyberpi.get_loudness`, `get_battery`, `cyberpi.controller.is_press`, `get_mac_address`

## MQTT-Protokoll

Jeder Roboter hat ein eigenes Topic-Präfix `lab/<robot_id>/`, zum Beispiel `lab/mbot-1/`. Befehle tragen eine ID, und der Roboter antwortet mit einem Ergebnis zur selben ID.

| Topic | Richtung | Retained | Inhalt |
| --- | --- | --- | --- |
| `cmd` | Server → Roboter | nein | Befehl mit ID, Name und Argumenten |
| `result` | Roboter → Server | nein | Ergebnis zu einem Befehl |
| `event` | Roboter → Server | nein | Ereignis bei Zustandswechsel |
| `state` | Roboter → Server | ja | aktueller Zustand |
| `status` | Roboter → Server | ja, als Last Will | `online` oder `offline` |
| `hello` | Roboter → Server | nein | Protokollversion, Skriptversion, MAC-Adresse beim Verbinden |
| `hb` | Server → Roboter | nein | Heartbeat, einmal pro Sekunde |

**Nachrichten**

```json
// cmd
{"id":"c-8f3a","name":"drive_until",
 "args":{"max_cm":200,"stop_below_cm":12,"stop_on_color":"blue"}}

// result
{"id":"c-8f3a","ok":true,"reason":"wall",
 "data":{"driven_cm":42,"distance_cm":11,"color":"white"}}

// event
{"kind":"obstacle_near","data":{"distance_cm":8},"seq":117,"t_ms":123456}

// state
{"skill":"idle","distance_cm":34,"color":"white","line":"none",
 "yaw":-2,"battery":81,"tilted":false,"seq":118,"t_ms":123460}

// hello
{"v":1,"script":"0.1.0","mac":"aa:bb:cc:dd:ee:ff"}
```

**Regeln**

- **QoS 0 überall.** Zuverlässigkeit entsteht über ID, Ergebnis und einen Timeout im Server. Der Timeout ist die Maximaldauer des Skills plus 2 Sekunden.
- **Doppelte Befehle:** Der Roboter merkt sich die letzten 16 IDs und ignoriert Wiederholungen.
- **Ein Bewegungs-Skill zur Zeit:** Ein neuer Bewegungsbefehl während einer Fahrt endet mit `ok:false` und `reason:"busy"`. `stop` wird immer angenommen.
- **Werte für `reason`:** `done`, `wall`, `color`, `max_cm`, `timeout`, `stopped`, `obstacle`, `busy`, `tilted`, `reassigned`, `error`.
- **Zeit:** Der Roboter hat keine Uhr. `t_ms` zählt Millisekunden seit dem Start, der Server stempelt die Ankunftszeit.
- **Größe:** kompaktes JSON, Nachrichten unter 512 Byte.
- **Keepalive:** 30 Sekunden. Der Last Will setzt `status` auf `offline`.
- **Zugang:** Benutzername ist die Roboter-ID, dazu ein Passwort pro Roboter. Ein Roboter darf nur unter seinem eigenen Präfix lesen und schreiben.
- **Version:** `hello.v` ist die Protokollversion. Bei einer unbekannten Version nimmt der Server den Roboter nicht in Betrieb und zeigt das im Leitstand an.

`docs/protocol.md` im Repo ist die verbindliche Fassung. Server, Roboterskript und Simulator werden gegen dieselben Beispielnachrichten getestet.

## Tool-Katalog

Die Agenten bekommen wenige, mächtige Tools mit eingebauter Abbruchbedingung statt einer Eins-zu-eins-Abbildung der Hersteller-API. Das Modell entscheidet an den Ecken, der Roboter fährt dazwischen selbst.

| Gruppe | Tool | Parameter | Ergebnis |
| --- | --- | --- | --- |
| Bewegen | `drive` | `cm`, `speed?` | `driven_cm`, `reason` |
| Bewegen | `turn` | `degrees`, positiv ist rechts | `turned_deg`, `reason` |
| Bewegen | `drive_until` | `max_cm`, `stop_below_cm?`, `stop_on_color?` | `driven_cm`, `reason`, `distance_cm`, `color` |
| Bewegen | `follow_line_until` | `stop_on_color?`, `max_cm` | `driven_cm`, `reason`, `color` |
| Bewegen | `retrace_path` | keine | `segments`, `reason` |
| Bewegen | `stop` | keine | `ok` |
| Wahrnehmen | `read_sensors` | keine | `distance_cm`, `color`, `line`, `yaw`, `battery`, `tilted` |
| Wahrnehmen | `wait_for` | `event`, `timeout_s` | `event`, `data` oder `reason:"timeout"` |
| Wahrnehmen | `list_robots` | keine | Name, online, Akku, Farbe je Roboter |
| Ausdrücken | `set_leds` | `color` oder `pattern` | `ok` |
| Ausdrücken | `show_text` | `text`, höchstens 60 Zeichen | `ok` |
| Ausdrücken | `play_sound` | `name` | `ok` |
| Ausdrücken | `play_melody` | `notes` als Liste aus Tonhöhe und Dauer | `ok` |
| Ausdrücken | `gesture` | `kind`: `nod`, `shake`, `spin` | `ok` |
| Ausdrücken | `identify` | keine | `ok` |
| Reden | `say` | `to`, `text` bis 60 Zeichen, `emotion?` | `delivered` |
| Reden | `tell_team` | `text` | `ok` |
| Merken | `remember` | `text` | `note_id` |
| Merken | `forget` | `note_id` | `ok` |
| Abschluss | `finish` | `report` | beendet den Lauf |

**Regeln**

- **Strukturierte Ergebnisse mit Grund:** Jedes Bewegungs-Tool liefert `reason`. Daran hängt die Entscheidungsfähigkeit des Agenten.
- **Roboter-Parameter nur bei Bedarf:** Gehört dem Besitzer genau ein Roboter, haben die Tools keinen Parameter `robot`. Bei mehreren kommt `robot` als Auswahl aus den erlaubten Aliasnamen dazu, und `stop` akzeptiert zusätzlich `"all"`.
- **Aliasnamen statt Geräte-IDs:** Der Agent kennt `ich`, `links`, `rechts`, nie `mbot-2`. Geräte lassen sich so tauschen, ohne Prompts zu ändern.
- **`say` ist ein Paket:** Text aufs Display des Sprechers, Licht, Ton, optional Geste, und die Nachricht landet im Postfach des Empfängers. `to` ist ein Agentenname oder `"all"`.
- **`finish` liefert den Bericht**, den das Team später mit dem Protokoll vergleicht.
- **Gleichzeitiger Start:** Für Choreografien mit mehreren Robotern kommt später `run_together(steps)` dazu, das der Server zeitgleich verschickt.
- **Bewusst kein Tool:** WLAN, Cloud-Dienste des Herstellers, Neustart, Name setzen, rohe Motorleistung.

Die Tools liegen in einer internen Registry mit Name, JSON-Schema und Handler. Der Agent ruft sie direkt auf, der optionale MCP-Endpunkt stellt dieselbe Registry nach außen bereit.

## Server in Go

Der Server ist ein einzelnes Binary `labserver`, das Broker, Roboter-Registry, Agenten, Modellzugriff, Speicherung und Weboberfläche enthält.

**Module**

| Paket | Aufgabe |
| --- | --- |
| `config` | lädt YAML, Geheimnisse nur aus Umgebungsvariablen, prüft die Konfiguration beim Start |
| `broker` | eingebetteter MQTT-Broker mit TCP- und TLS-Listener, Anmeldung und Zugriffsregeln pro Roboter |
| `robots` | kennt jeden Roboter, seinen Zustand, seinen Besitzer; sendet Heartbeats; ordnet Ergebnisse den Befehlen zu |
| `tools` | Registry und Implementierungen; übersetzt Tool-Aufrufe in MQTT-Befehle und wartet auf das Ergebnis |
| `agent` | ein Aktor pro Team, siehe Abschnitt Agent-Runtime |
| `llm` | Client für `/chat/completions` mit Tools, mehrere Anbieter, Warteschlange, Token- und Kostenzählung |
| `memory` | Notizen und Zusammenfassung älterer Verlaufsteile |
| `store` | SQLite und Migrationen |
| `web` | Teamseite, Leitstand, Chat-Wand, SSE |
| `auth` | Team-Links, Admin-Passwort, Roboter-Zugänge |
| `mcp` | optionaler Endpunkt, späterer Meilenstein |

**Bibliotheken, Versionen bitte aktuell prüfen**

- `github.com/mochi-mqtt/server/v2` als Broker
- `modernc.org/sqlite` als SQLite-Treiber ohne cgo
- `gopkg.in/yaml.v3` für die Konfiguration
- `golang.org/x/crypto/acme/autocert` für Zertifikate
- `github.com/modelcontextprotocol/go-sdk` nur für den MCP-Endpunkt
- Sonst die Standardbibliothek: `net/http`, `html/template`, `embed`, `log/slog`, `context`
- Kein SDK für das Sprachmodell. Ein schlanker HTTP-Client reicht und hält die Anbieter austauschbar.

**Konfiguration, Beispiel**

```yaml
server:
  http_addr: ":443"
  mqtt_tls_addr: ":8883"
  mqtt_plain_addr: ""          # nur lokal, z. B. ":1883"
  tls: { mode: autocert, hosts: [lab.example.org, mqtt.example.org] }
  admin_password_env: LAB_ADMIN_PASSWORD
  db_path: /var/lib/labserver/lab.db

llm:
  default: mistral
  providers:
    mistral:
      base_url: https://api.mistral.ai/v1
      api_key_env: MISTRAL_API_KEY
      model: mistral-small-latest   # für Wettkämpfe eine feste Version eintragen
    ollama:
      base_url: http://localhost:11434/v1
      model: "<lokales Modell mit Tool-Calling>"
  max_parallel_calls: 5

budgets:
  max_steps_per_wakeup: 12
  max_tokens_per_run: 200000
  max_agent_messages_per_minute: 6

robots:
  - { id: mbot-1, password_env: MBOT1_PASSWORD, color: red }
  - { id: mbot-2, password_env: MBOT2_PASSWORD, color: blue }

teams:
  - { id: rot,  name: "Team Rot",  token_env: TEAM_ROT_TOKEN }
  - { id: blau, name: "Team Blau", token_env: TEAM_BLAU_TOKEN }

profile: einzel
profiles:
  einzel:
    rot:  { ich: mbot-1 }
    blau: { ich: mbot-2 }
  duo:
    rot:  { links: mbot-1, rechts: mbot-2 }
```

**Besitz von Robotern**

- Gespeichert wird pro Roboter der Besitzer, nicht pro Besitzer eine Liste. Die Exklusivregel kann so nicht verletzt werden.
- Ein Profil ist eine komplette Aufteilung. Es wird beim Laden als Ganzes geprüft, bei einem doppelt vergebenen Roboter startet der Server nicht.
- Roboter ohne Besitzer stehen still und zeigen „frei“.
- Umhängen ist ein atomarer Schritt: stoppen, Fahrtenlog und Zustand zurücksetzen, Besitzer wechseln, neuen Besitzer anzeigen. Ein laufender Befehl endet mit `reassigned`.
- Besitzer ist entweder der interne Agent eines Teams oder eine MCP-Verbindung, nie beides.

**Modellzugriff**

- Eine Schnittstelle `Provider` mit einer Methode für einen Chat-Aufruf mit Tools. Mistral, andere OpenAI-kompatible Anbieter und Ollama unterscheiden sich nur in der Konfiguration.
- Eine Warteschlange begrenzt parallele Aufrufe und verteilt reihum auf die Teams.
- Jeder Aufruf wird mit Tokenzahl, Dauer und geschätzten Kosten gespeichert.
- Bei Fehlern des Anbieters: begrenzte Wiederholung mit Pause, dann Meldung an das Team. Ein konfigurierter Rückfall-Anbieter ist möglich.
- Das Modell pro Team oder pro Lauf ist einstellbar. Das ermöglicht das Modell-Rennen.

**Datenmodell in SQLite**

| Tabelle | Inhalt |
| --- | --- |
| `prompts` | Team, Version, Text, Zeitpunkt |
| `runs` | Team, Prompt-Version, Modell, Start, Ende, Endgrund, Token, Kosten |
| `messages` | Lauf, Reihenfolge, Quelle, Art, Text, Daten, Zeitpunkt |
| `tool_calls` | Lauf, Reihenfolge, Name, Argumente, Ergebnis, Dauer |
| `llm_calls` | Lauf, Modell, Token ein und aus, Dauer |
| `notes` | Team, Text, Herkunft, Zeitpunkt |
| `assignments` | Roboter, Besitzer, Alias |

**HTTP-Routen**

| Route | Zweck |
| --- | --- |
| `GET /t/{token}` | Teamseite |
| `POST /t/{token}/prompt` | Prompt deployen, optional mit Neustart |
| `POST /t/{token}/message` | Nachricht an den eigenen Agenten |
| `POST /t/{token}/start`, `/stop` | Lauf starten oder anhalten |
| `GET /t/{token}/events` | SSE: Protokoll, Zustand, Kosten |
| `GET/POST/DELETE /t/{token}/notes` | Notizen ansehen, ändern, löschen |
| `GET /admin` und Unterseiten | Leitstand |
| `POST /admin/stop-all` | Nothalt für alle Roboter |
| `GET /wall/{token}` | Chat-Wand für den Beamer |
| `GET /healthz` | Betriebsprüfung |
| `/mcp/{connection}` | optionaler MCP-Endpunkt |

**MCP als zweite Tür, später**

- Streamable HTTP über das offizielle Go-SDK.
- Jede Verbindung hat ein Token und eine exklusive Menge von Robotern aus dem Profil.
- Die Tool-Liste wird pro Verbindung aus der Registry gebaut. Ob das SDK eine Server-Instanz pro Anfrage unterstützt, ist gegen die aktuelle Doku zu prüfen.
- Externe Clients erfahren Ereignisse nur über `wait_for` und ein Feld `events_since_last_call` im Tool-Ergebnis.

## Agent-Runtime

Jedes Team hat genau einen Agenten: eine Goroutine mit Postfach, die nur dann das Modell aufruft, wenn eine Nachricht eintrifft. Alles, was passiert, ist eine Nachricht.

**Umschlag**

```go
type Source string // "robot", "team", "agent:<name>", "system"

type Message struct {
    ID   string
    From Source
    Kind string          // event | instruction | chat | deploy | control
    Text string
    Data json.RawMessage
    At   time.Time
}
```

**Zustände:** `wartet`, `denkt`, `handelt`. Im Zustand `wartet` wird das Modell nicht aufgerufen. Der Zustand geht an die Teamseite und an den Roboter, der beim Denken pulsiert.

**Schleife**

1. Auf Nachrichten warten und alle vorliegenden bündeln.
2. Kontext bauen.
3. Modell aufrufen.
4. Verlangte Tools ausführen, Ergebnisse anhängen.
5. Vor dem nächsten Modellaufruf neu eingetroffene Nachrichten einfügen.
6. Wiederholen, bis das Modell kein Tool mehr verlangt oder eine Endbedingung greift. Dann zurück zu `wartet`.

**Kontext für einen Modellaufruf**

- **System:** feste Plattformregeln, dann der Team-Prompt in seiner aktuellen Version, dann die Notizen.
- **Verlauf:** Zusammenfassung älterer Teile plus die letzten Runden im Wortlaut.
- **Neue Nutzernachricht:** ein Block „Posteingang“ mit allen neuen Nachrichten samt Absender und Alter, dazu der aktuelle Roboterzustand.

**Die Quelle bestimmt das Vertrauen**

| Quelle | Bedeutung für den Agenten |
| --- | --- |
| Team | Anweisungen, denen er folgt |
| Roboter | Tatsachen über die Welt |
| Andere Agenten | Behauptungen, die geprüft werden wollen |
| System | Regeln der Plattform, etwa Budget oder Rundenende |

Die Kennzeichnung im Posteingang ist eindeutig und nicht fälschbar: Ein fremder Agent kann nicht als Team auftreten. Ob der Agent eine fremde Nachricht trotzdem wie einen Befehl behandelt, ist Gegenstand der Prompt-Injection-Übung.

**Unterbrechung**

- Normale Nachrichten warten bis zum nächsten Modellaufruf.
- Sicherheitsereignisse des Roboters und ein Stopp vom Team oder Leitstand haben Vorrang. Stopp bricht den laufenden Modellaufruf über den `context` ab und stoppt den Roboter.

**Prompts deployen**

- Jedes Deploy erzeugt eine neue Version. Alte Versionen bleiben erhalten.
- Wirksam wird sie an der nächsten Rundengrenze, nie mitten in einem Tool-Aufruf.
- „Deploy“ behält Verlauf und Notizen. „Deploy und Neustart“ beginnt einen neuen Lauf.
- Jeder Lauf speichert seine Prompt-Version. Teams können Versionen vergleichen und zurückrollen.

**Gedächtnis in drei Schichten**

| Schicht | Wer schreibt | Sieht das Modell | Zweck |
| --- | --- | --- | --- |
| Arbeitskontext | Runtime | ja, gleitendes Fenster plus Zusammenfassung | der aktuelle Lauf |
| Notizen | der Agent über `remember`, das Team auf seiner Seite | ja, in jeder Runde, mit Größenlimit | Lernen über Läufe hinweg |
| Protokoll | Runtime | nein | Lügendetektor, Wiederholung, Auswertung |

- Notizen sind für das Team sichtbar, änderbar und löschbar. Vor Wettkämpfen setzt der Leitstand sie für alle zurück.
- Jede Notiz speichert ihre Herkunft: aus welchem Lauf und nach welcher Nachricht sie entstand. So lässt sich zeigen, wenn ein fremder Agent eine falsche Notiz untergeschoben hat.

**Budgets und Schutz vor Schleifen**

- Höchstzahl an Schritten pro Aufwachen.
- Höchstzahl an Token pro Lauf.
- Höchstzahl an Nachrichten an andere Agenten pro Minute.
- Wird ein Budget erreicht, bekommt der Agent eine Systemnachricht und das Team einen Hinweis.

**Endbedingungen eines Laufs**

- Der Agent ruft `finish(report)` auf.
- Schritt-, Token- oder Zeitlimit.
- Stopp durch Team oder Leitstand.

**Gespräche zwischen Agenten**

- `say(to, text)` legt eine Nachricht ins Postfach des Empfängers. Das weckt dessen Agenten.
- Ein Moderator im Server regelt die Reihenfolge, wahlweise reihum oder frei mit Budget. Der Modus ist pro Szenario einstellbar.
- Jedes Szenario definiert ein klares Ende, etwa eine Abstimmung oder ein erreichtes Feld.
