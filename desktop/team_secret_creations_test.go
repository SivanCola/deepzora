package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"deepzora/internal/config"
	"deepzora/internal/teamroster"
)

func TestRejectedTeamSecretNeverWritesPlaintext(t *testing.T) {
	for _, kind := range []string{"purpose", "capacity", "retired-capacity"} {
		t.Run(kind, func(t *testing.T) {
			slots := config.TeamSecretSlots{Dir: filepath.Join(t.TempDir(), "slots")}
			roster := teamroster.Roster{}
			member := roster.AddAgent(teamroster.Agent{Name: "Ada"})
			book := teamroster.TeamSecretBook{}
			req := SaveTeamSecretRequest{RequestID: "reject-invalid-secret", Operation: "save", Name: "DEPLOY", Value: "never-write-this", Agents: []string{member.ID}}
			switch kind {
			case "purpose":
				req.Purpose = strings.Repeat("x", 801)
			case "capacity":
				for i := range 128 {
					book.Secrets = append(book.Secrets, teamroster.TeamSecret{Name: fmt.Sprintf("KEY_%d", i), Slot: "existing", Agents: []string{member.ID}})
				}
			case "retired-capacity":
				book.Secrets = []teamroster.TeamSecret{{Name: "DEPLOY", Slot: "current", Agents: []string{member.ID}}}
				for i := range 2048 {
					book.Retired = append(book.Retired, teamroster.RetiredTeamSecret{Name: "DEPLOY", Slot: fmt.Sprintf("old-%d", i)})
				}
			}
			if err := applyUserTeamSecret(t.Context(), &book, roster, slots, req, filepath.Join(t.TempDir(), "intents")); err == nil {
				t.Fatal("invalid metadata accepted")
			}
			if _, err := os.Stat(slots.Dir); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("rejected request touched credential directory: %v", err)
			}
		})
	}
}

func TestTeamSecretCreationRecoveryUsesCommittedBook(t *testing.T) {
	for _, outcome := range []string{"unpublished", "publish_failed", "committed", "retired", "pending_deletion", "unknown", "corrupt"} {
		t.Run(outcome, func(t *testing.T) {
			store := teamroster.Store{Dir: t.TempDir()}
			slots := config.TeamSecretSlots{Dir: t.TempDir()}
			var slot string
			_, err := store.MutateTeamSecrets(t.Context(), 0, "create-slot-intent", "digest", func(book *teamroster.TeamSecretBook) error {
				var err error
				slot, err = slots.SaveWithIntent(t.Context(), "private-value", teamSecretIntentDir(store))
				if err != nil {
					return err
				}
				if outcome == "unpublished" {
					return errors.New("injected metadata publication failure")
				}
				if outcome == "publish_failed" {
					// Block the actual atomic rename after slot publication.
					if err := os.Mkdir(filepath.Join(store.Dir, teamroster.SecretFile), 0700); err != nil {
						return err
					}
				}
				book.Secrets = []teamroster.TeamSecret{{Name: "DEPLOY", Slot: slot, Agents: []string{}}}
				if outcome == "retired" {
					book.Retired = []teamroster.RetiredTeamSecret{{Name: "DEPLOY", Slot: slot}}
					book.Secrets = nil
				}
				if outcome == "pending_deletion" {
					book.PendingDeletion = []string{slot}
					book.Secrets = nil
				}
				return nil
			})
			unpublished := outcome == "unpublished" || outcome == "publish_failed"
			if (err != nil) != unpublished {
				t.Fatal(err)
			}
			if outcome == "publish_failed" {
				if err := os.Remove(filepath.Join(store.Dir, teamroster.SecretFile)); err != nil {
					t.Fatal(err)
				}
			}
			if outcome == "unknown" || outcome == "corrupt" {
				body := `{"schemaVersion":999}`
				if outcome == "corrupt" {
					body = "{broken"
				}
				if err := os.WriteFile(filepath.Join(store.Dir, teamroster.SecretFile), []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			// Recreate owners to simulate restart after plaintext publication or
			// after a successful metadata commit whose result was never observed.
			err = recoverTeamSecretCreations(t.Context(), teamroster.Store{Dir: store.Dir}, config.TeamSecretSlots{Dir: slots.Dir})
			blocked := outcome == "unknown" || outcome == "corrupt"
			if (err != nil) != blocked {
				t.Fatalf("recovery error = %v", err)
			}
			_, valueErr := slots.Read(slot)
			if (valueErr != nil) != unpublished {
				t.Fatalf("recovery kept/deleted wrong credential: %v", valueErr)
			}
			_, intentErr := os.Stat(filepath.Join(teamSecretIntentDir(store), slot))
			if (intentErr == nil) != blocked {
				t.Fatalf("wrong intent retention: %v", intentErr)
			}
			if !blocked {
				if err := recoverTeamSecretCreations(t.Context(), store, slots); err != nil {
					t.Fatalf("repeated recovery: %v", err)
				}
			}
		})
	}
}
