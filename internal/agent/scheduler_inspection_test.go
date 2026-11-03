package agent

func (s *SubagentScheduler) ActiveWriterClaims() []WritePathSet {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []WritePathSet
	for _, v := range s.active {
		if v {
			out = append(out, WritePathSet{WholeWorkspace: true})
		}
	}
	for range s.parents {
		out = append(out, WritePathSet{WholeWorkspace: true})
	}
	return out
}

func testWriterReservation(root string) (WritePathSet, error) {
	return WritePathSet{WholeWorkspace: true, WorkspaceRoot: root}, nil
}
