package service

import (
	"deepzora/internal/config"
	"deepzora/internal/stats"
	"errors"
	"slices"
	"time"
)

type TeamUsageRequest struct {
	From    string `json:"from"`
	To      string `json:"to"`
	AgentID string `json:"agentId"`
}

func (a *App) GetTeamUsage(req TeamUsageRequest) ([]stats.TeamUsage, error) {
	from, e1 := time.ParseInLocation("2006-01-02", req.From, time.Local)
	to, e2 := time.ParseInLocation("2006-01-02", req.To, time.Local)
	if e1 != nil || e2 != nil || from.After(to) || to.Sub(from) > 366*24*time.Hour {
		return nil, errors.New("Team usage requires a date range of at most 366 days")
	}
	rows, err := stats.NewWriter(config.StatsDir()).QueryTeam(stats.SourceFilter{From: from, To: to})
	if err != nil {
		return nil, err
	}
	if req.AgentID != "" {
		rows = slices.DeleteFunc(rows, func(row stats.TeamUsage) bool { return row.AgentID != req.AgentID })
	}
	return rows, nil
}
