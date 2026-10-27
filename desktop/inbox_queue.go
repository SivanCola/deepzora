package main

import (
	"deepzora/internal/control"
	"deepzora/internal/sessioninbox"
)

type InboxQueueResultView struct {
	Outcome  string                     `json:"outcome"`
	Reason   string                     `json:"reason,omitempty"`
	Snapshot InboxSnapshotView          `json:"snapshot"`
	Edit     *control.InboxQueueEdit    `json:"edit,omitempty"`
	Receipt  *sessioninbox.InboxReceipt `json:"receipt,omitempty"`
}

func (a *App) InboxQueueForTarget(target InboxTargetView, request control.InboxQueueRequest) (InboxQueueResultView, error) {
	a.runtimeAdmissionMu.RLock()
	defer a.runtimeAdmissionMu.RUnlock()
	current, err := a.captureInboxTarget(target.TabID, target.SessionPath)
	if err != nil || current != target {
		return InboxQueueResultView{Outcome: "unavailable", Reason: "session_changed"}, nil
	}
	ctrl, err := a.inboxCtrl(target.TabID)
	if err != nil {
		return InboxQueueResultView{}, err
	}
	api, ok := ctrl.(interface {
		InboxQueue(string, control.InboxQueueRequest) (control.InboxQueueResult, error)
	})
	if !ok {
		return InboxQueueResultView{Outcome: "unavailable", Reason: "unsupported"}, nil
	}
	result, err := api.InboxQueue(target.SessionPath, request)
	if err != nil {
		return InboxQueueResultView{}, inboxBridgeError(err)
	}
	return inboxQueueView(result), nil
}

func inboxQueueView(result control.InboxQueueResult) InboxQueueResultView {
	return InboxQueueResultView{Outcome: result.Outcome, Reason: result.Reason, Snapshot: inboxSnapshotView(result.Snapshot), Edit: result.Edit, Receipt: result.Receipt}
}
