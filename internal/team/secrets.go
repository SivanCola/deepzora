package team

import (
	"context"
	"errors"
	"slices"
	"strings"

	"deepzora/internal/agent"
	"deepzora/internal/teamroster"
)

type SecretSlots interface {
	Save(context.Context, string) (string, error)
	Read(string) (string, error)
}

var ErrSecretsUnavailable = errors.New("Team secrets are unavailable until complete masking is active")

func (s *Service) SecretsAvailable() bool {
	return s != nil && s.secretSlots != nil && s.secretsReady != nil && s.secretsReady()
}

// CommandSecretEnvironment reloads metadata for every launch and reads only
// authorized referenced slots. A new agent never inherits another agent's grant.
func (s Seat) CommandSecretEnvironment(ctx context.Context, kind, executable, command string) ([]string, error) {
	if !strings.Contains(command, teamroster.SecretEnvPrefix) {
		return nil, nil
	}
	if !s.Service.SecretsAvailable() {
		return nil, ErrSecretsUnavailable
	}
	if agent.SubagentDepth(ctx) > 0 {
		return nil, errors.New("task subagents cannot borrow a Team secret identity")
	}
	r, err := s.Service.store.Load()
	if err != nil {
		return nil, err
	}
	if _, ok := r.Agent(s.Ref.Agent); !ok {
		return nil, teamroster.ErrUnknownAgent
	}
	var names []string
	switch kind {
	case "bash", "zsh", "sh":
		names, err = ReferencedPOSIXSecrets(command)
	case "powershell":
		names, err = ReferencedPowerShellSecrets(ctx, executable, command)
	default:
		return nil, errors.New("this shell cannot receive Team secrets; use POSIX direct variables or PowerShell $env:DEEPZORA_SECRET_NAME")
	}
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, nil
	}
	book, err := s.Service.store.TeamSecrets()
	if err != nil {
		return nil, err
	}
	env := make([]string, 0, len(names))
	for _, name := range names {
		i := slices.IndexFunc(book.Secrets, func(sc teamroster.TeamSecret) bool { return sc.Name == name })
		if i < 0 || !slices.Contains(book.Secrets[i].Agents, s.Ref.Agent) {
			return nil, errors.New("this member is not authorized for a referenced Team secret; request authorization in its private chat")
		}
		value, err := s.Service.secretSlots.Read(book.Secrets[i].Slot)
		if err != nil {
			return nil, errors.New("referenced Team secret credential slot is unavailable")
		}
		env = append(env, teamroster.SecretEnvPrefix+name+"="+value)
	}
	return env, nil
}

func (s *Service) RequestSecret(ctx context.Context, c Caller, name, purpose, requestID string) error {
	if !s.SecretsAvailable() {
		return ErrSecretsUnavailable
	}
	if c.Group != "" {
		return errors.New("request a secret in your own private Team conversation")
	}
	if Normalize := teamroster.NormalizeSecretName(name); Normalize == "" || Normalize != name {
		return teamroster.ErrInvalid
	}
	if len(purpose) > 800 || len(requestID) < 8 || len(requestID) > 128 {
		return teamroster.ErrInvalid
	}
	s.configuration.RLock()
	defer s.configuration.RUnlock()
	roster, err := s.store.Load()
	if err != nil {
		return err
	}
	if _, ok := roster.Agent(c.Agent); !ok {
		return teamroster.ErrUnknownAgent
	}
	book, err := s.store.TeamSecrets()
	if err != nil {
		return err
	}
	_, err = s.store.MutateTeamSecrets(ctx, book.Revision, c.Agent+":"+requestID, name+":"+purpose, func(book *teamroster.TeamSecretBook) error {
		for _, request := range book.Requests {
			if request.Agent == c.Agent && request.Name == name {
				return nil
			}
		}
		count := 0
		for _, request := range book.Requests {
			if request.Agent == c.Agent {
				count++
			}
		}
		if count >= 32 {
			return teamroster.ErrInvalid
		}
		book.Requests = append(book.Requests, teamroster.TeamSecretRequest{ID: teamroster.NewSecretRequestID(), Agent: c.Agent, Name: name, Purpose: purpose, At: s.now()})
		return nil
	})
	if err == nil {
		s.changed()
	}
	return err
}
