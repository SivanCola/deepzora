package main

import (
	"errors"
	"fmt"
	"strings"

	"deepzora/internal/sessioninbox"
)

// inboxTransientCode is the bridge code for a transient inbox-target fence;
// the renderer keeps the message queued and retries it automatically.
const inboxTransientCode = "inbox_target_transient"

// errInboxTargetChanged is the transient identity/route fence: the tab is
// switching, reconnecting or re-aiming, and the same call succeeds once it
// settles. Boundaries classify it as inbox_target_transient.
var errInboxTargetChanged = errors.New("inbox target changed")

// InboxTargetView fences follow-ups across both local replacement and remote
// selection changes. It is process-local and never written to the inbox ledger.
type InboxTargetView struct {
	TabID       string `json:"tabId"`
	SessionPath string `json:"sessionPath"`
	Generation  uint64 `json:"generation"`
	Selection   uint64 `json:"selection"`
}

// Callers hold App.mu. Canonical tabs deliberately have no legacy file path;
// read their owned identity without calling back into a controller under the lock.
func inboxTabIdentity(tab *WorkspaceTab) string {
	if id := strings.TrimSpace(tab.SessionID); id != "" {
		return sessionRoute(id)
	}
	return ""
}

func (a *App) CaptureInboxTarget(tabID, expectedPath string) (InboxTargetView, error) {
	a.runtimeAdmissionMu.RLock()
	defer a.runtimeAdmissionMu.RUnlock()
	target, err := a.captureInboxTarget(tabID, expectedPath)
	if errors.Is(err, errInboxTargetChanged) {
		return InboxTargetView{}, inboxTargetTransient(err)
	}
	return target, err
}

func (a *App) captureInboxTarget(tabID, expectedPath string) (InboxTargetView, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, tab := range a.tabs {
		if tab.ID == tabID && tab.Ctrl != nil && expectedPath != "" && inboxTabIdentity(tab) == expectedPath {
			return InboxTargetView{TabID: tabID, SessionPath: expectedPath, Generation: tab.SessionGeneration}, nil
		}
	}
	return InboxTargetView{}, errInboxTargetChanged
}

// EnqueueInboxFollowupForTarget writes through the captured session owner.
func (a *App) EnqueueInboxFollowupForTarget(target InboxTargetView, display, submit string, invocations []InvocationRequest, key string) (InboxReceiptView, error) {
	if strings.TrimSpace(key) == "" {
		return InboxReceiptView{}, inboxNotSubmitted(fmt.Errorf("idempotency key required"))
	}

	a.runtimeAdmissionMu.RLock()
	defer a.runtimeAdmissionMu.RUnlock()
	current, err := a.captureInboxTarget(target.TabID, target.SessionPath)
	if err != nil || current != target {
		if err != nil && !errors.Is(err, errInboxTargetChanged) {
			return InboxReceiptView{}, inboxNotSubmitted(err)
		}
		return InboxReceiptView{}, inboxTargetTransient(errInboxTargetChanged)
	}
	ctrl, err := a.inboxCtrl(target.TabID)
	if err != nil {
		return InboxReceiptView{}, inboxNotSubmitted(err)
	}
	return a.enqueueInboxWithController(target.TabID, ctrl, sessioninbox.IntentFollowup, display, submit, invocations, key, false, "", target.SessionPath)
}

func inboxNotSubmitted(err error) error {
	return &inboxCodedError{code: "inbox_not_submitted", cause: err}
}

// LookupInboxFollowupForTarget never creates an item, including on a missing
// or expired receipt. Position zero identifies an already removed item.
func (a *App) LookupInboxFollowupForTarget(target InboxTargetView, key string) (InboxReceiptView, error) {
	a.runtimeAdmissionMu.RLock()
	defer a.runtimeAdmissionMu.RUnlock()
	owner, err := a.localReceiptTarget(target.SessionPath)
	if err != nil {
		return InboxReceiptView{}, err
	}
	reader, ok := owner.ctrl.(interface {
		LookupInboxReceiptForSession(string, string) (sessioninbox.InboxReceipt, bool, error)
	})
	if !ok {
		return InboxReceiptView{}, fmt.Errorf("inbox receipt lookup unavailable")
	}
	receipt, found, err := reader.LookupInboxReceiptForSession(owner.path, key)
	if err != nil {
		return InboxReceiptView{}, err
	}
	if !found {
		return InboxReceiptView{}, fmt.Errorf("inbox receipt unconfirmed")
	}
	if !a.localReceiptOwnerCurrent(owner) {
		return InboxReceiptView{}, fmt.Errorf("inbox receipt owner changed during read")
	}
	return InboxReceiptView{ItemID: receipt.ItemID, Disposition: string(receipt.Disposition), Position: receipt.Position, Paused: receipt.Paused, Idempotent: receipt.Idempotent}, nil
}
