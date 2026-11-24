// Package lab holds the rules of the lab: teams join with the lab code, the
// control room assigns robots to teams, and teams name their robots. It keeps
// the database, the robot registry and the live logs consistent.
package lab

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
	"unicode/utf8"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/config"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/live"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/robots"
	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

// UserError is a mistake the person can fix. Its text is German and shown as is.
type UserError struct {
	Msg      string
	Conflict bool // the request clashes with existing data (HTTP 409)
}

func (e *UserError) Error() string { return e.Msg }

func userErr(format string, args ...any) error {
	return &UserError{Msg: fmt.Sprintf(format, args...)}
}

// ErrUnknownTeam and ErrUnknownRobot are returned for IDs that do not exist.
var (
	ErrUnknownTeam  = errors.New("unknown team")
	ErrUnknownRobot = errors.New("unknown robot")
)

// TeamColors are the colors a team can get, in the order they are handed out.
// They match the robot's LED palette (robot/src/hw.py).
var TeamColors = []string{"blue", "green", "orange", "purple", "red", "cyan", "yellow"}

var colorNames = map[string]string{
	"red": "rot", "blue": "blau", "green": "grün", "yellow": "gelb",
	"purple": "lila", "cyan": "türkis", "orange": "orange", "white": "weiß",
}

const (
	labCodeKey      = "lab_code"
	labCodeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789" // no 0/O, 1/I/L
	labCodeLength   = 4
	maxTeamName     = 30
	maxAlias        = 16
)

var aliasPattern = regexp.MustCompile(`^\p{L}[\p{L}\p{N}_-]*$`)

// Service is safe for concurrent use.
type Service struct {
	Store  *store.Store
	Robots *robots.Registry
	Hub    *live.Hub
	Config *config.Config
	Log    *slog.Logger

	// OnTeamEvent, if set, also passes notices about a team's robots and name
	// to the team's agent (it decides itself whether that wakes it).
	OnTeamEvent func(teamID, kind, text string)
	// OnTaskClosed, if set, ends running runs and open conversations in a
	// task that the control room closed.
	OnTaskClosed func(taskID string)

	mu        sync.Mutex // serializes changes to teams, assignments and model choices
	labCode   string
	allowed   map[string]bool // providers teams may choose besides the default
	modelsGen atomic.Int64

	conversationOff atomic.Bool
}

// Start loads the lab code and restores robot assignments from the database.
func (s *Service) Start(ctx context.Context) error {
	code := s.Config.Server.LabCode
	if code == "" {
		stored, err := s.Store.Setting(ctx, labCodeKey)
		if err != nil {
			return err
		}
		code = stored
	}
	if code == "" {
		code = newLabCode()
	}
	if err := s.Store.SetSetting(ctx, labCodeKey, code); err != nil {
		return err
	}
	s.labCode = code
	if err := s.loadAllowed(ctx); err != nil {
		return err
	}
	if err := s.loadConversation(ctx); err != nil {
		return err
	}

	for _, r := range s.Config.Robots {
		if err := s.Store.SeeRobot(ctx, r.ID); err != nil {
			return err
		}
	}

	teams, err := s.teamMap(ctx)
	if err != nil {
		return err
	}
	as, err := s.Store.Assignments(ctx)
	if err != nil {
		return err
	}
	for _, a := range as {
		t, ok := teams[a.TeamID]
		if !ok || !s.robotExists(a.RobotID) {
			// Robot removed from the config or team gone: drop the stale row.
			_ = s.Store.Assign(ctx, a.RobotID, "", "")
			continue
		}
		_ = s.Robots.Assign(a.RobotID, robots.Owner{TeamID: t.ID, TeamName: t.Name, Color: t.Color, Alias: a.Alias})
	}
	return nil
}

// LabCode returns the current code teams need to join.
func (s *Service) LabCode() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.labCode
}

// NewLabCode replaces the lab code. Teams that already joined keep their links.
func (s *Service) NewLabCode(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	code := newLabCode()
	if err := s.Store.SetSetting(ctx, labCodeKey, code); err != nil {
		return "", err
	}
	s.labCode = code
	return code, nil
}

// Join creates a team if code is the current lab code.
func (s *Service) Join(ctx context.Context, code, name string) (store.Team, error) {
	name, err := cleanTeamName(name)
	if err != nil {
		return store.Team{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	code = strings.ToUpper(strings.TrimSpace(code))
	if subtle.ConstantTimeCompare([]byte(code), []byte(s.labCode)) != 1 {
		return store.Team{}, userErr("Der Lab-Code stimmt nicht. Er steht beim Leitstand oder auf dem Beamer.")
	}
	color, err := s.nextColor(ctx)
	if err != nil {
		return store.Team{}, err
	}
	t, err := s.Store.CreateTeam(ctx, store.Team{ID: "t" + randomString(6), Name: name, Token: randomString(20), Color: color})
	if errors.Is(err, store.ErrConflict) {
		return store.Team{}, &UserError{Msg: fmt.Sprintf("Den Namen „%s“ hat schon ein anderes Team. Wählt einen anderen.", name), Conflict: true}
	}
	if err != nil {
		return store.Team{}, err
	}
	s.Record(ctx, store.Event{Kind: store.EventTeamJoined, Actor: store.ActorTeam, Team: t.ID, Data: map[string]string{"name": t.Name}})
	if err := s.bridgeDefaultTask(ctx, t.ID); err != nil {
		return store.Team{}, err
	}
	s.Log.Info("team joined", "team", t.ID, "name", t.Name)
	return t, nil
}

// TeamByToken resolves a team link.
func (s *Service) TeamByToken(ctx context.Context, token string) (store.Team, error) {
	return s.Store.TeamByToken(ctx, token)
}

// RenameTeam gives a team a new name and updates its robots' displays.
func (s *Service) RenameTeam(ctx context.Context, teamID, name string) error {
	name, err := cleanTeamName(name)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	err = s.Store.RenameTeam(ctx, teamID, name)
	switch {
	case errors.Is(err, store.ErrConflict):
		return &UserError{Msg: fmt.Sprintf("Den Namen „%s“ hat schon ein anderes Team.", name), Conflict: true}
	case errors.Is(err, store.ErrNotFound):
		return ErrUnknownTeam
	case err != nil:
		return err
	}
	t, _ := s.Store.Team(ctx, teamID)
	for _, rb := range s.Robots.Snapshot() {
		if rb.Owner == teamID {
			_ = s.Robots.Assign(rb.ID, robots.Owner{TeamID: t.ID, TeamName: t.Name, Color: t.Color, Alias: rb.Alias})
		}
	}
	s.notify(teamID, "team_renamed", fmt.Sprintf("Euer Team heißt jetzt „%s“.", name), map[string]string{"name": name})
	return nil
}

// SetTeamColor changes a team's color. Its robots switch their LEDs at the
// next heartbeat. Several teams may share a color.
func (s *Service) SetTeamColor(ctx context.Context, teamID, color string) error {
	if !slices.Contains(TeamColors, color) {
		return &UserError{Msg: "Diese Farbe gibt es nicht."}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.Store.SetTeamColor(ctx, teamID, color); errors.Is(err, store.ErrNotFound) {
		return ErrUnknownTeam
	} else if err != nil {
		return err
	}
	t, _ := s.Store.Team(ctx, teamID)
	for _, rb := range s.Robots.Snapshot() {
		if rb.Owner == teamID {
			_ = s.Robots.Assign(rb.ID, robots.Owner{TeamID: t.ID, TeamName: t.Name, Color: t.Color, Alias: rb.Alias})
		}
	}
	s.notify(teamID, "team_color", fmt.Sprintf("Eure Teamfarbe ist jetzt %s.", colorNames[color]), map[string]string{"color": color})
	return nil
}

// DeleteTeam removes a team, stops and frees its robots and invalidates its link.
func (s *Service) DeleteTeam(ctx context.Context, teamID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	task := s.activeTaskID(ctx, teamID)
	if err := s.Store.DeleteTeam(ctx, teamID); errors.Is(err, store.ErrNotFound) {
		return ErrUnknownTeam
	} else if err != nil {
		return err
	}
	for _, rb := range s.Robots.Snapshot() {
		if rb.Owner == teamID {
			_ = s.Robots.Assign(rb.ID, robots.Owner{})
			s.Record(ctx, store.Event{Kind: store.EventRobotReleased, Actor: store.ActorAdmin, Team: teamID, Task: task, Robot: rb.ID,
				Data: map[string]string{"alias": rb.Alias}})
		}
	}
	s.Hub.Forget(teamID)
	s.Log.Info("team deleted", "team", teamID)
	return nil
}

// AssignRobot gives a robot to a team ("" frees it). A newly assigned robot is
// named after its color until the team renames it.
func (s *Service) AssignRobot(ctx context.Context, robotID, teamID string) error {
	cfgRobot, ok := s.configRobot(robotID)
	if !ok {
		return ErrUnknownRobot
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var prev robots.Robot
	for _, rb := range s.Robots.Snapshot() {
		if rb.ID == robotID {
			prev = rb
		}
	}
	if prev.Owner == teamID {
		return nil
	}

	owner := robots.Owner{}
	if teamID != "" {
		t, err := s.Store.Team(ctx, teamID)
		if errors.Is(err, store.ErrNotFound) {
			return ErrUnknownTeam
		} else if err != nil {
			return err
		}
		alias := s.freeAlias(teamID, cfgRobot)
		owner = robots.Owner{TeamID: t.ID, TeamName: t.Name, Color: t.Color, Alias: alias}
	}
	if err := s.Store.Assign(ctx, robotID, teamID, owner.Alias); err != nil {
		return err
	}
	if err := s.Robots.Assign(robotID, owner); err != nil {
		return err
	}
	if prev.Owner != "" {
		s.Record(ctx, store.Event{Kind: store.EventRobotReleased, Actor: store.ActorAdmin, Team: prev.Owner, Task: s.activeTaskID(ctx, prev.Owner),
			Robot: robotID, Data: map[string]string{"alias": prev.Alias}})
		s.notify(prev.Owner, "robot_removed", fmt.Sprintf("Der Leitstand hat euch %s weggenommen.", prev.Alias),
			map[string]string{"robot": robotID, "alias": prev.Alias})
	}
	if teamID != "" {
		s.Record(ctx, store.Event{Kind: store.EventRobotAssigned, Actor: store.ActorAdmin, Team: teamID, Task: s.activeTaskID(ctx, teamID),
			Robot: robotID, Data: map[string]string{"alias": owner.Alias}})
		s.notify(teamID, "robot_added", fmt.Sprintf("Ihr habt einen Roboter dazubekommen: %s (%s). Ihr könnt ihn umbenennen.", owner.Alias, robotID),
			map[string]string{"robot": robotID, "alias": owner.Alias})
	}
	return nil
}

// RenameRobot changes what a team calls one of its robots.
func (s *Service) RenameRobot(ctx context.Context, teamID, robotID, alias string) error {
	alias = strings.TrimSpace(alias)
	if err := validAlias(alias); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var rb *robots.Robot
	for _, r := range s.Robots.Snapshot() {
		if r.ID == robotID && r.Owner == teamID {
			rb = &r
		}
	}
	if rb == nil {
		return ErrUnknownRobot
	}
	if rb.Alias == alias {
		return nil
	}
	if err := s.Store.Assign(ctx, robotID, teamID, alias); errors.Is(err, store.ErrConflict) {
		return &UserError{Msg: fmt.Sprintf("Ihr habt schon einen Roboter namens „%s“.", alias), Conflict: true}
	} else if err != nil {
		return err
	}
	t, err := s.Store.Team(ctx, teamID)
	if err != nil {
		return err
	}
	if err := s.Robots.Assign(robotID, robots.Owner{TeamID: t.ID, TeamName: t.Name, Color: t.Color, Alias: alias}); err != nil {
		return err
	}
	s.Record(ctx, store.Event{Kind: store.EventRobotRenamed, Actor: store.ActorTeam, Team: teamID, Task: s.activeTaskID(ctx, teamID),
		Robot: robotID, Data: map[string]string{"alias": alias, "old": rb.Alias}})
	s.notify(teamID, "robot_renamed", fmt.Sprintf("%s heißt jetzt %s.", rb.Alias, alias), map[string]string{"robot": robotID, "alias": alias, "old": rb.Alias})
	return nil
}

// --- helpers ---------------------------------------------------------------

func (s *Service) notify(teamID, kind, text string, data any) {
	raw, _ := json.Marshal(data)
	s.Hub.Publish(teamID, live.LogEntry{Source: live.SourceSystem, Kind: kind, Text: text, Data: raw})
	if s.OnTeamEvent != nil {
		s.OnTeamEvent(teamID, kind, text)
	}
}

func (s *Service) teamMap(ctx context.Context) (map[string]store.Team, error) {
	teams, err := s.Store.Teams(ctx)
	if err != nil {
		return nil, err
	}
	m := map[string]store.Team{}
	for _, t := range teams {
		m[t.ID] = t
	}
	return m, nil
}

func (s *Service) configRobot(id string) (config.Robot, bool) {
	for _, r := range s.Config.Robots {
		if r.ID == id {
			return r, true
		}
	}
	return config.Robot{}, false
}

func (s *Service) robotExists(id string) bool {
	_, ok := s.configRobot(id)
	return ok
}

// freeAlias picks the robot's color name, numbered if the team already uses it.
func (s *Service) freeAlias(teamID string, r config.Robot) string {
	base := colorNames[r.Color]
	if base == "" {
		base = r.ID
	}
	taken := map[string]bool{}
	for _, rb := range s.Robots.Snapshot() {
		if rb.Owner == teamID {
			taken[strings.ToLower(rb.Alias)] = true
		}
	}
	alias := base
	for n := 2; taken[strings.ToLower(alias)]; n++ {
		alias = fmt.Sprintf("%s%d", base, n)
	}
	return alias
}

// nextColor returns the least used team color.
func (s *Service) nextColor(ctx context.Context) (string, error) {
	teams, err := s.Store.Teams(ctx)
	if err != nil {
		return "", err
	}
	used := map[string]int{}
	for _, t := range teams {
		used[t.Color]++
	}
	best := TeamColors[0]
	for _, c := range TeamColors {
		if used[c] < used[best] {
			best = c
		}
	}
	return best, nil
}

func cleanTeamName(name string) (string, error) {
	name = strings.Join(strings.Fields(name), " ")
	n := utf8.RuneCountInString(name)
	if n < 2 {
		return "", userErr("Der Teamname braucht mindestens 2 Zeichen.")
	}
	if n > maxTeamName {
		return "", userErr("Der Teamname darf höchstens %d Zeichen haben.", maxTeamName)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", userErr("Der Teamname enthält ungültige Zeichen.")
		}
	}
	return name, nil
}

func validAlias(alias string) error {
	switch {
	case alias == "":
		return userErr("Gebt dem Roboter einen Namen.")
	case utf8.RuneCountInString(alias) > maxAlias:
		return userErr("Der Robotername darf höchstens %d Zeichen haben.", maxAlias)
	case !aliasPattern.MatchString(alias):
		return userErr("Robotername: ein Wort, das mit einem Buchstaben beginnt. Erlaubt sind Buchstaben, Ziffern, - und _.")
	case strings.EqualFold(alias, "all") || strings.EqualFold(alias, "alle"):
		return userErr("„%s“ ist reserviert, weil es alle Roboter meint.", alias)
	}
	return nil
}

var b32 = base32.NewEncoding("abcdefghijkmnpqrstuvwxyz23456789").WithPadding(base32.NoPadding)

// randomString returns n random characters from an unambiguous alphabet.
func randomString(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return b32.EncodeToString(buf)[:n]
}

func newLabCode() string {
	buf := make([]byte, labCodeLength)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	for i, b := range buf {
		buf[i] = labCodeAlphabet[int(b)%len(labCodeAlphabet)]
	}
	return string(buf)
}
