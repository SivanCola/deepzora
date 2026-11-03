package session

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"deepzora/internal/agent"
	"deepzora/internal/provider"
	"deepzora/internal/sessioncontent"
)

type UserEvidence struct {
	Sequence  uint64
	MessageID string
	Text      string
}

type UserEvidencePage struct {
	Lines      []UserEvidence
	Generation string
	Through    uint64
	More       bool
}

// ReadUserEvidenceAfter reads a bounded set of the earliest unreviewed human
// turns through the canonical history reader. Tool/assistant bodies are never
// loaded. Origin, not a display name or text delimiter, establishes authorship.
func (q *Query) ReadUserEvidenceAfter(ctx context.Context, ref SessionRef, after uint64, generation string, since time.Time, limit int) (UserEvidencePage, error) {
	if q == nil || q.persistence == nil {
		return UserEvidencePage{}, fmt.Errorf("memory evidence requires a session query")
	}
	if err := ref.validate(q.hostID); err != nil {
		return UserEvidencePage{}, err
	}
	if err := q.prepareMemoryEvidence(ctx, ref); err != nil {
		return UserEvidencePage{}, err
	}
	limit = max(1, min(limit, 32))
	page, err := q.ReadHistoryWindow(ctx, ref, HistoryWindowRequest{Anchor: "newest", Limit: 32})
	if err != nil {
		return UserEvidencePage{}, err
	}
	if page.Status != "ready" {
		return UserEvidencePage{}, fmt.Errorf("memory history is %s", page.Status)
	}
	if generation != "" && generation != page.Generation {
		return UserEvidencePage{}, ErrStaleGeneration
	}
	result := UserEvidencePage{Generation: page.Generation, Through: page.SnapshotSequence, Lines: []UserEvidence{}}
	for range 4096 {
		reached := false
		for _, entry := range page.Messages {
			if entry.EventSequence <= after {
				reached = true
				continue
			}
			if entry.Role != string(provider.RoleUser) {
				continue
			}
			message, err := q.memoryEvidenceMessage(ctx, ref, entry)
			if err != nil {
				return UserEvidencePage{}, err
			}
			if !agent.IsUserAuthoredTurnMessage(message) {
				continue
			}
			if !since.IsZero() && (message.CreatedAt == 0 || time.UnixMilli(message.CreatedAt).Before(since)) {
				continue
			}
			result.Lines = append(result.Lines, UserEvidence{Sequence: entry.EventSequence, MessageID: entry.MessageID, Text: agent.UserMessageText(message)})
		}
		slices.SortFunc(result.Lines, func(a, b UserEvidence) int {
			if a.Sequence < b.Sequence {
				return -1
			}
			if a.Sequence > b.Sequence {
				return 1
			}
			return 0
		})
		if len(result.Lines) > limit {
			result.Lines = result.Lines[:limit]
			result.More = true
		}
		if reached || !page.HasOlder {
			if result.More && len(result.Lines) > 0 {
				result.Through = result.Lines[len(result.Lines)-1].Sequence
			}
			return result, nil
		}
		page, err = q.ReadHistoryWindow(ctx, ref, HistoryWindowRequest{Anchor: "cursor", Cursor: page.OlderCursor, Limit: 32})
		if err != nil {
			return UserEvidencePage{}, err
		}
		if page.Status != "ready" || page.Generation != result.Generation {
			return UserEvidencePage{}, ErrStaleGeneration
		}
	}
	return UserEvidencePage{}, fmt.Errorf("memory evidence history exceeds bounded scan budget")
}

func (q *Query) prepareMemoryEvidence(ctx context.Context, ref SessionRef) error {
	filesystem, ok := q.persistence.(*FilesystemPersistence)
	if !ok {
		return nil
	}
	preparation := q.prepareHistoryLocator(filesystem, ref.SessionID, historyIndexPath(filepath.Clean(filesystem.Root), ref.SessionID), ctx)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-preparation.done:
		return preparation.err
	}
}

func (q *Query) memoryEvidenceMessage(ctx context.Context, ref SessionRef, entry PersistentMessage) (provider.Message, error) {
	body := entry.Inline
	if entry.ContentRef != nil {
		if entry.ContentRef.Bytes > sessioncontent.MaxReadRange {
			return provider.Message{}, fmt.Errorf("memory evidence exceeds body read budget")
		}
		body = nil
		for offset := int64(0); offset < entry.ContentRef.Bytes; {
			chunk, err := q.ReadContent(ctx, ref, *entry.ContentRef, offset, min(1<<20, entry.ContentRef.Bytes-offset))
			if err != nil {
				return provider.Message{}, err
			}
			if len(chunk) == 0 {
				return provider.Message{}, fmt.Errorf("incomplete memory evidence body")
			}
			body = append(body, chunk...)
			offset += int64(len(chunk))
		}
	}
	var message provider.Message
	if err := json.Unmarshal(body, &message); err != nil {
		return message, err
	}
	return message, nil
}
