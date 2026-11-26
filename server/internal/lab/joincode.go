package lab

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/store"
)

// Join codes: another PC of a team joins without typing the 20-character
// team link. A PC that has the link asks for a code; it is short (six
// characters without look-alikes), lives ten minutes and is kept only in
// memory. The team link stays the team's secret; a code only leads to it
// while it lives.

const (
	JoinCodeLength = 6
	JoinCodeTTL    = 10 * time.Minute
	// joinCodeRenew: a code with less time left is replaced by a new one.
	joinCodeRenew = 3 * time.Minute
)

type joinCode struct {
	team  string
	until time.Time
}

type joinCodes struct {
	mu    sync.Mutex
	codes map[string]joinCode
	now   func() time.Time
}

func (j *joinCodes) clock() time.Time {
	if j.now != nil {
		return j.now()
	}
	return time.Now()
}

// JoinCode returns a join code for the team and when it ends; a code that
// still has a while to live is shown again, so all PCs show the same one.
func (s *Service) JoinCode(teamID string) (string, time.Time) {
	j := &s.joins
	j.mu.Lock()
	defer j.mu.Unlock()
	now := j.clock()
	if j.codes == nil {
		j.codes = map[string]joinCode{}
	}
	for code, c := range j.codes {
		if !now.Before(c.until) {
			delete(j.codes, code)
		} else if c.team == teamID && c.until.Sub(now) > joinCodeRenew {
			return code, c.until
		}
	}
	for {
		code := newJoinCode()
		if _, taken := j.codes[code]; !taken {
			c := joinCode{team: teamID, until: now.Add(JoinCodeTTL)}
			j.codes[code] = c
			return code, c.until
		}
	}
}

// ErrJoinCode: the code is unknown or has expired.
var ErrJoinCode = userErr("Diesen Code gibt es nicht oder er ist abgelaufen. Lasst euch auf einem Rechner eures Teams einen neuen zeigen.")

// TeamByJoinCode resolves a join code to its team. Spaces, dashes and small
// letters do not matter.
func (s *Service) TeamByJoinCode(ctx context.Context, code string) (store.Team, error) {
	code = strings.ToUpper(strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(code)))
	j := &s.joins
	j.mu.Lock()
	c, ok := j.codes[code]
	j.mu.Unlock()
	if !ok || !j.clock().Before(c.until) {
		return store.Team{}, ErrJoinCode
	}
	t, err := s.Store.Team(ctx, c.team)
	if errors.Is(err, store.ErrNotFound) {
		return store.Team{}, ErrJoinCode
	}
	return t, err
}

func newJoinCode() string {
	buf := make([]byte, JoinCodeLength)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	for i, b := range buf {
		buf[i] = labCodeAlphabet[int(b)%len(labCodeAlphabet)]
	}
	return string(buf)
}
