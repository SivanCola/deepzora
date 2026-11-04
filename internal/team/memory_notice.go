package team

import (
	"context"
	"fmt"

	"deepzora/internal/teamroster"
)

func MemoryReviewNote(report ReviewReport) string {
	return fmt.Sprintf("Memory review: %d added, %d updated, %d archived. Review the entries in Team settings.", report.Added, report.Edited, report.Removed)
}
func (s *Service) RecordMemoryChange(ctx context.Context, agent, group, note string) error {
	if group == "" {
		return nil
	}
	_, err := s.store.AppendLine(ctx, group, teamroster.GroupLine{From: agent, Kind: teamroster.LineEvent, Event: teamroster.EventMemory, Text: note, At: s.now()})
	if err == nil {
		s.changed()
	}
	return err
}
