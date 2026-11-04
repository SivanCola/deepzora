package control

import (
	"errors"
	"strings"
	"sync"

	"deepzora/internal/hostframe"
	"deepzora/internal/team"
	"deepzora/internal/tool"
)

// TeamSeat is a host-owned binding, never selected by model arguments.
type TeamSeat interface {
	Projection() (team.Projection, error)
	Team() (*team.Service, team.SeatRef)
}

type TeamOptions struct {
	Seat              TeamSeat
	Link              *tool.TeamLink
	PresetFingerprint string
}

type teamBinding struct {
	mu                sync.Mutex
	seat              TeamSeat
	turn              teamTurn
	presetFingerprint string
}

func (b *teamBinding) bound() TeamSeat {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.seat
}

func (c *Controller) startTeam(options TeamOptions) {
	c.team.seat = options.Seat
	c.team.presetFingerprint = options.PresetFingerprint
	if options.Link != nil {
		options.Link.Bind(teamDirectory{c})
	}
}

// BoundTeamSeat preserves the exact owner across controller rebuilds.
func (c *Controller) BoundTeamSeat() TeamSeat { return c.team.bound() }

// TeamPresetFingerprint describes the definition this runtime actually built.
func (c *Controller) TeamPresetFingerprint() string {
	c.team.mu.Lock()
	defer c.team.mu.Unlock()
	return c.team.presetFingerprint
}

func (c *Controller) withTeamTail(input string) (string, error) {
	seat := c.team.bound()
	if seat == nil {
		return input, nil
	}
	p, err := seat.Projection()
	if err != nil {
		return "", err
	}
	if p.Identity == "" {
		return "", errors.New("team seat has no current identity")
	}
	var tail strings.Builder
	for _, part := range []struct{ tag, text string }{
		{"agent-identity", p.Identity}, {"team-roster", p.Roster},
		{"group-roster", p.Group},
		{"shared-memory", p.Memory},
	} {
		text := part.text
		if part.tag == "group-roster" && text == "" {
			text = "This seat is not in a current group."
		}
		if text != "" {
			tail.WriteString(hostframe.Block(part.tag, nil, text))
		}
	}
	return input + "\n\n" + tail.String(), nil
}
