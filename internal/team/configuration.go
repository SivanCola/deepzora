package team

import "deepzora/internal/teamroster"

func authorizeCreate(r *teamroster.Roster, c Caller) (*teamroster.Agent, error) {
	a, ok := r.Agent(c.Agent)
	if !ok {
		return nil, teamroster.ErrUnknownAgent
	}
	if !a.Main {
		return nil, ErrNotMain
	}
	return a, nil
}

func authorizeUpdate(r *teamroster.Roster, c Caller, target string) (*teamroster.Agent, error) {
	self, ok := r.Agent(c.Agent)
	if !ok {
		return nil, teamroster.ErrUnknownAgent
	}
	a := self
	if target != "" {
		if a, ok = agentByRef(r, target); !ok {
			return nil, teamroster.ErrUnknownAgent
		}
	}
	if a.ID != self.ID && !self.Main {
		return nil, ErrSelfOnly
	}
	return a, nil
}

// CheckCreateAgent authorizes the caller before the host presents any approval card.
func (s *Service) CheckCreateAgent(c Caller) error {
	r, err := s.store.Load()
	if err != nil {
		return err
	}
	_, err = authorizeCreate(&r, c)
	return err
}

// AgentForUpdate resolves an authorized target to its immutable id before approval.
func (s *Service) AgentForUpdate(c Caller, target string) (teamroster.Agent, error) {
	a, _, err := s.AgentForUpdateAt(c, target)
	return a, err
}

// AgentForUpdateAt resolves identity and revision from one authorized snapshot.
func (s *Service) AgentForUpdateAt(c Caller, target string) (teamroster.Agent, uint64, error) {
	r, err := s.store.Load()
	if err != nil {
		return teamroster.Agent{}, 0, err
	}
	a, err := authorizeUpdate(&r, c, target)
	if err != nil {
		return teamroster.Agent{}, 0, err
	}
	return *a, r.Revision, nil
}

// RequiresApproval identifies changes that affect a seat's future instructions or execution.
func (ch AgentChange) RequiresApproval(a teamroster.Agent) bool {
	return ch.Role != nil && *ch.Role != a.Role ||
		ch.Instructions != nil && *ch.Instructions != a.Instructions ||
		ch.Model != nil && a.SessionPath == "" && *ch.Model != a.InitialModel ||
		ch.Folder != nil && *ch.Folder != "" && *ch.Folder != a.Folder
}
