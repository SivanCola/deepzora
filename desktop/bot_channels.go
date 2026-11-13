package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"deepzora/desktop/internal/botchannels"
	"deepzora/internal/config"
	"deepzora/internal/control"
	"deepzora/internal/event"
	"deepzora/internal/filelock"
)

type BotConnectionInput struct {
	botchannels.Config
	Secret  string `json:"secret"`
	LoginID string `json:"loginId"`
	Connect bool   `json:"connect,omitempty"`
}
type BotSessionView struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Workspace string `json:"workspace"`
}
type BotChannelsView struct {
	Connections []botchannels.View `json:"connections"`
	Sessions    []BotSessionView   `json:"sessions"`
	Models      []BotModelView     `json:"models"`
	Workspaces  []BotWorkspaceView `json:"workspaces"`
	Members     []BotMemberView    `json:"members"`
}
type botChannelState struct {
	logins         map[string]*botLoginAttempt
	op             sync.Mutex
	mu             sync.Mutex
	runs           map[string]*botChannelRun
	closed         bool
	adapterFactory func(botchannels.Saved, *http.Client) botchannels.Adapter
	migrationError string
}
type botChannelRun struct {
	inboundMu                  sync.Mutex
	receipts                   *botchannels.Receipts
	mu                         sync.Mutex
	config                     botchannels.Saved
	status, message            string
	cancel                     context.CancelFunc
	done                       chan struct{}
	output                     chan string
	events                     chan event.Event
	ctrl                       *control.Controller
	sink                       *tabEventSink
	generation                 uint64
	tabID                      string
	prompts                    map[string]control.PromptIdentity
	received, sent             int
	lastReceivedAt, lastSentAt string
	routes                     map[string]string // accepted submission -> canonical session
	turnRoutes                 map[string]string // turn -> canonical session
	promptOwners               map[string]*control.Controller
	sourceOwners               map[string]*control.Controller
}

func (a *App) botStore() botchannels.Store {
	return botchannels.Store{Dir: filepath.Join(config.DeepzoraHomeDir(), "bot-channels")}
}
func (a *App) ListBotChannels() (BotChannelsView, error) {
	entries, err := a.botStore().Read()
	if err != nil {
		return BotChannelsView{}, err
	}
	result := BotChannelsView{Connections: []botchannels.View{}, Sessions: []BotSessionView{}}
	result.Members, err = a.botMembers()
	if err != nil {
		return result, err
	}
	result.Models, err = a.botModelCatalog()
	if err != nil {
		return result, err
	}
	state, err := a.workspaceRegistry().Load(a.bootContext())
	if err != nil {
		return result, err
	}
	result.Workspaces = []BotWorkspaceView{{ID: "global", Title: "Global"}}
	for _, id := range state.WorkspaceIDs {
		w := state.Workspaces[id]
		if id != "global" && w.Visible {
			result.Workspaces = append(result.Workspaces, BotWorkspaceView{ID: id, Title: w.Title, Root: w.Root})
		}
	}
	a.botChannels.mu.Lock()
	for _, e := range entries {
		v := botchannels.View{Config: e.Config, HasSecret: e.Secret != "", Status: "stopped", Enabled: e.Enabled}
		if e.MemberID != "" {
			if m, err := botTeamMember(e.MemberID); err == nil {
				v.MemberName = m.Name
				v.ModelRef = m.InitialModel
				if id, ok := parseSessionRoute(m.SessionPath); ok {
					v.SessionID = id
				}
			} else {
				v.ModelError = err.Error()
			}
		} else if a.botChannels.migrationError != "" {
			v.Error = a.botChannels.migrationError
			v.Status = "error"
		}
		if v.Mode == "" {
			v.Mode = "follow"
		}
		if r := a.botChannels.runs[e.ID]; r != nil {
			r.mu.Lock()
			v.Status, v.Error = r.status, r.message
			v.Received, v.Sent = r.received, r.sent
			v.LastReceivedAt, v.LastSentAt = r.lastReceivedAt, r.lastSentAt
			r.mu.Unlock()
		}
		result.Connections = append(result.Connections, v)
	}
	a.botChannels.mu.Unlock()
	a.mu.RLock()
	defer a.mu.RUnlock()
	for i := range result.Connections {
		v := &result.Connections[i]
		for _, t := range a.runtimeTabsLocked() {
			if t.SessionID == v.SessionID {
				if ctrl, ok := t.Ctrl.(*control.Controller); ok {
					v.ActualModel = ctrl.ModelRef()
					v.ModelError = botModelFailure(ctrl)
				}
			}
		}
	}
	for _, id := range a.tabOrder {
		t := a.tabs[id]
		if t == nil || !t.Ready || t.ReadOnly || t.SessionID == "" || isDedicatedBotSession(t.SessionID) {
			continue
		}
		if _, ok := t.Ctrl.(*control.Controller); !ok {
			continue
		}
		result.Sessions = append(result.Sessions, BotSessionView{ID: t.SessionID, Title: t.TopicTitle, Workspace: t.WorkspaceRoot})
	}
	return result, nil
}
func (a *App) DeleteBotConnection(id string) error {
	a.botChannels.op.Lock()
	defer a.botChannels.op.Unlock()
	if err := a.stopBotChannel(id); err != nil {
		return err
	}
	var editUnlock func()
	defer func() {
		if editUnlock != nil {
			editUnlock()
		}
	}()
	return a.botStore().Edit(func(entries []botchannels.Saved) ([]botchannels.Saved, error) {
		if idx := slices.IndexFunc(entries, func(e botchannels.Saved) bool { return e.ID == id }); idx >= 0 {
			var err error
			editUnlock, err = a.lockBotAccount(entries[idx])
			if err != nil {
				return nil, err
			}
		}
		return slices.DeleteFunc(entries, func(e botchannels.Saved) bool { return e.ID == id }), nil
	})
}
func (a *App) DisconnectBotChannel(id string) error {
	a.botChannels.op.Lock()
	defer a.botChannels.op.Unlock()
	return a.setBotEnabledLocked(id, false)
}
func (a *App) stopBotChannel(id string) error {
	a.botChannels.mu.Lock()
	r := a.botChannels.runs[id]
	a.botChannels.mu.Unlock()
	if r == nil {
		return nil
	}
	r.cancel()
	select {
	case <-r.done:
	case <-time.After(10 * time.Second):
		return errors.New("bot is still stopping; retry shortly")
	}
	a.botChannels.mu.Lock()
	delete(a.botChannels.runs, id)
	a.botChannels.mu.Unlock()
	return nil
}
func (a *App) ConnectBotChannel(id string) error {
	a.botChannels.op.Lock()
	defer a.botChannels.op.Unlock()
	if err := a.setBotEnabledLocked(id, true); err != nil {
		return err
	}
	return a.startSavedBotLocked(id, false)
}

func (a *App) connectBotChannelLocked(id string, restoring bool) error {
	a.botChannels.mu.Lock()
	closed := a.botChannels.closed
	a.botChannels.mu.Unlock()
	if closed || a.shuttingDown.Load() {
		return errors.New("desktop is shutting down")
	}
	if err := a.stopBotChannel(id); err != nil {
		return err
	}
	entries, err := a.botStore().Read()
	if err != nil {
		return err
	}
	idx := slices.IndexFunc(entries, func(e botchannels.Saved) bool { return e.ID == id })
	if idx < 0 {
		return errors.New("bot connection not found")
	}
	entry := entries[idx]
	if err := botchannels.Validate(entry.Config); err != nil {
		return err
	}
	r := &botChannelRun{config: entry, status: "connecting", done: make(chan struct{}), events: make(chan event.Event, 128), output: make(chan string, 64), prompts: map[string]control.PromptIdentity{}}
	if entry.MemberID != "" {
		if _, err := botTeamMember(entry.MemberID); err != nil {
			return err
		}
	} else if entry.Mode == "dedicated" {
		if err := a.validateBotModel(entry.Config); err != nil {
			return err
		}
	} else if !restoring {
		if err := a.bindBotRun(r); err != nil {
			return err
		}
	}
	r.receipts = &botchannels.Receipts{Path: filepath.Join(a.botStore().Dir, botAccountKey(entry)+".receipts.json")}
	unlock, err := a.lockBotAccount(entry)
	if err != nil {
		return err
	}
	// A different process may have edited the binding while we acquired its
	// account lock. Refuse the stale snapshot before starting network workers.
	current, err := a.botStore().Read()
	if err != nil || !slices.ContainsFunc(current, func(c botchannels.Saved) bool { return c == entry }) {
		unlock()
		return errors.New("bot configuration changed; refresh and reconnect")
	}
	ctx, cancel := context.WithCancel(a.bootContext())
	r.cancel = cancel
	a.botChannels.mu.Lock()
	if a.botChannels.runs == nil {
		a.botChannels.runs = map[string]*botChannelRun{}
	}
	a.botChannels.runs[id] = r
	a.botChannels.mu.Unlock()
	go func() {
		defer close(r.done)
		defer unlock()
		defer cancel()
		if restoring && entry.Mode != "dedicated" && entry.MemberID == "" {
			prepareCtx, prepareCancel := context.WithTimeout(ctx, 90*time.Second)
			err := a.waitForBotSession(prepareCtx, r)
			prepareCancel()
			if err != nil {
				if ctx.Err() == nil {
					r.setStatus("error", err.Error())
				}
				return
			}
		}
		if entry.Mode == "dedicated" {
			prepareCtx, prepareCancel := context.WithTimeout(ctx, 90*time.Second)
			err := a.prepareDedicatedBotSession(prepareCtx, entry.Config)
			prepareCancel()
			if err == nil {
				err = a.bindBotRun(r)
			}
			if err != nil {
				if ctx.Err() != nil {
					r.setStatus("stopped", "")
				} else {
					r.setStatus("error", err.Error())
				}
				return
			}
		}
		root := ""
		if entry.MemberID != "" {
			prepared, err := a.prepareBotMember(ctx, entry.MemberID)
			if err != nil {
				r.setStatus("error", err.Error())
				return
			}
			root = prepared.ctrl.WorkspaceRoot()
		} else {
			root = r.ctrl.WorkspaceRoot()
		}
		client, err := botchannels.HTTPClient(a.networkProxySpecForRoot(root))
		if err != nil {
			r.setStatus("error", err.Error())
			return
		}
		defer client.CloseIdleConnections()
		factory := a.botChannels.adapterFactory
		if factory == nil {
			factory = botchannels.NewAdapter
		}
		a.runBotChannel(ctx, r, factory(entry, client))
	}()
	return nil
}

func botAccountKey(entry botchannels.Saved) string {
	identity := entry.Provider + ":" + entry.Secret
	if entry.Provider == "feishu" || entry.Provider == "lark" {
		identity = entry.Provider + ":" + entry.AppID
	}
	hash := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(hash[:])
}
func (a *App) lockBotAccount(entry botchannels.Saved) (func(), error) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	unlock, err := filelock.Acquire(ctx, filepath.Join(a.botStore().Dir, botAccountKey(entry)+".lock"))
	if err != nil {
		return nil, errors.New("this bot is connected in another window or process; disconnect it there first")
	}
	return unlock, nil
}
func (r *botChannelRun) setStatus(status, message string) {
	r.mu.Lock()
	if r.status == "error" && status != "error" {
		r.mu.Unlock()
		return
	}
	r.status, r.message = status, message
	r.mu.Unlock()
}
func (a *App) botBindingValid(r *botChannelRun) bool {
	if r.config.MemberID != "" {
		_, err := botTeamMember(r.config.MemberID)
		return err == nil
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.botRuntimeTabLocked(r) != nil
}

// A canonical runtime can move between visible and detached tabs. The
// controller, sink and immutable session identity are the authority, not the
// process-local frontend tab ID or its presentation generation.
func (a *App) botRuntimeTabLocked(r *botChannelRun) *WorkspaceTab {
	if r.ctrl == nil {
		return nil
	}
	if ref, bound := r.ctrl.SessionRef(); !bound || ref.SessionID != r.config.SessionID {
		return nil
	}
	for _, t := range a.runtimeTabsLocked() {
		if t.SessionID == r.config.SessionID && t.Ctrl == r.ctrl && t.sink == r.sink && !t.ReadOnly && !t.removed &&
			(r.config.Mode != "dedicated" || r.ctrl.ModelRef() == r.config.ModelRef) {
			return t
		}
	}
	return nil
}
func (r *botChannelRun) enqueue(text string) {
	if text == "" {
		return
	}
	if len([]rune(text)) > 32000 {
		text = string([]rune(text)[:32000]) + "\n[Reply truncated. Open Desktop for the complete response.]"
	}
	select {
	case r.output <- text:
	default:
		r.setStatus("error", "reply queue full; reconnect and inspect the Desktop transcript")
		r.cancel()
	}
}
func (a *App) runBotChannel(ctx context.Context, r *botChannelRun, adapter botchannels.Adapter) {
	var workers sync.WaitGroup
	ready := make(chan struct{})
	var readyOnce sync.Once
	workers.Go(func() {
		for retry := 0; ctx.Err() == nil; retry++ {
			err := adapter.Run(ctx, func(m botchannels.Incoming) error { return a.receiveBotMessage(ctx, r, m) }, func(status string) {
				r.setStatus(status, "")
				if status == "connected" {
					readyOnce.Do(func() { close(ready) })
				}
			})
			if ctx.Err() != nil {
				return
			}
			if !botchannels.Retryable(err) {
				message := "channel stopped; reconnect after checking credentials and permissions"
				if err != nil {
					message = err.Error()
				}
				r.setStatus("error", message)
				r.cancel()
				return
			}
			r.setStatus("reconnecting", err.Error())
			timer := time.NewTimer(time.Duration(1<<min(retry, 5)) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	})
	workers.Go(func() {
		for {
			select {
			case <-ctx.Done():
				return
			case text := <-r.output:
				select {
				case <-ctx.Done():
					return
				case <-ready:
				}
				if ctx.Err() != nil {
					return
				}
				if !a.botBindingValid(r) {
					r.setStatus("error", "session changed; reconnect")
					r.cancel()
					return
				}
				if err := adapter.Send(ctx, text); err != nil {
					if ctx.Err() == nil {
						r.setStatus("error", err.Error())
						r.cancel()
					}
					return
				}
				r.mu.Lock()
				r.sent++
				r.lastSentAt = time.Now().UTC().Format(time.RFC3339)
				r.mu.Unlock()
			}
		}
	})
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
		case <-ticker.C:
			if !a.botBindingValid(r) {
				r.setStatus("error", "session changed; reconnect")
				r.cancel()
			}
		case e := <-r.events:
			a.forwardBotEvent(r, e)
		}
	}
	workers.Wait()
	r.mu.Lock()
	if r.status != "error" {
		r.status = "stopped"
	}
	r.mu.Unlock()
}
func (a *App) receiveBotMessage(ctx context.Context, r *botChannelRun, m botchannels.Incoming) error {
	if m.ChatID != r.config.ChatID || m.SenderID != r.config.SenderID {
		return nil
	}
	r.inboundMu.Lock()
	defer r.inboundMu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !a.botBindingValid(r) {
		return errors.New("bound session is no longer open")
	}
	text := strings.TrimSpace(m.Text)
	if text == "" {
		r.enqueue("This channel currently accepts text only. Open Desktop for attachments.")
		return nil
	}
	if len(text) > 32000 || m.ID == "" {
		r.enqueue("Message is too large or has no stable identity.")
		return nil
	}
	claimed, err := r.receipts.Claim(m.ChatID + "\x00" + m.ID)
	if err != nil {
		return errors.New("cannot record incoming message receipt; inspect local bot storage")
	}
	if !claimed {
		return nil
	}
	r.mu.Lock()
	r.received++
	r.lastReceivedAt = time.Now().UTC().Format(time.RFC3339)
	r.mu.Unlock()
	fields := strings.Fields(text)
	bound := r
	if r.config.MemberID != "" {
		bound, err = a.prepareBotMember(ctx, r.config.MemberID)
		if err != nil {
			r.enqueue("Member could not start. Check its model, permissions and workspace in Team settings.")
			return nil
		}
	}
	switch fields[0] {
	case "/help", "/start":
		r.enqueue("Deepzora · shared session\nSend text to run a task. /stop cancels the active task. /approve ID and /deny ID answer an approval. Structured questions and attachments are handled in Desktop.")
		if r.config.MemberID == "" {
			r.ctrl.ReplayPendingPromptsTo(event.FuncSink(func(e event.Event) { a.forwardBotEvent(r, e) }))
		}
		return nil
	case "/stop":
		if r.config.MemberID != "" {
			state := bound.ctrl.RuntimeStateSnapshot()
			r.mu.Lock()
			owned := r.turnRoutes[state.TurnID] == bound.config.SessionID
			r.mu.Unlock()
			if !owned {
				r.enqueue("No task from this channel is running.")
				return nil
			}
		}
		bound.ctrl.CancelSessionFrom("bot_stop")
		r.enqueue("Cancellation requested.")
		return nil
	case "/approve", "/deny":
		if len(fields) != 2 {
			r.enqueue("Use /approve ID or /deny ID from the approval message.")
			return nil
		}
		r.mu.Lock()
		identity, ok := r.prompts[fields[1]]
		owner := r.ctrl
		if r.config.MemberID != "" {
			owner = r.promptOwners[fields[1]]
		}
		r.mu.Unlock()
		if !ok || owner == nil {
			r.enqueue("Approval expired or unavailable. Open Desktop.")
			return nil
		}
		if err := owner.ResolvePromptExact(identity, control.PromptAnswer{Allow: fields[0] == "/approve"}); err != nil {
			r.enqueue("Approval is no longer available. Open Desktop.")
		} else {
			r.mu.Lock()
			delete(r.prompts, fields[1])
			r.mu.Unlock()
			r.enqueue("Approval recorded.")
		}
		return nil
	}
	if strings.HasPrefix(text, "/") {
		r.enqueue("Unknown channel command. Use /help; Desktop commands are not accepted here.")
		return nil
	}
	digest := sha256.Sum256([]byte(r.config.Provider + "\x00" + r.config.ID + "\x00" + m.ChatID + "\x00" + m.ID))
	requestID := "bot-" + hex.EncodeToString(digest[:])
	if r.config.MemberID != "" {
		r.mu.Lock()
		if r.routes == nil {
			r.routes = map[string]string{}
			r.turnRoutes = map[string]string{}
			r.sourceOwners = map[string]*control.Controller{}
			r.promptOwners = map[string]*control.Controller{}
		}
		r.routes[requestID] = bound.config.SessionID
		r.sourceOwners[requestID] = bound.ctrl
		r.mu.Unlock()
	}
	err = a.submitBotTurn(ctx, bound, text, requestID)
	if err != nil {
		if r.config.MemberID != "" {
			r.mu.Lock()
			delete(r.routes, requestID)
			delete(r.sourceOwners, requestID)
			r.mu.Unlock()
		}
		r.enqueue("The session did not accept this message. Check Desktop for a running task, model configuration or recovery prompt.")
	}
	return nil
}

func (a *App) submitBotTurn(ctx context.Context, r *botChannelRun, text, id string) error {
	// Bot 请求必须通过桌面唯一 admission 边界，避免绕过模型应用、会话替换
	// 和 turn 完成时的保留窗口；check 只允许连接时捕获的同一个 owner。
	// Retry only a pre-admission tab handoff, never an accepted submission.
	for range 3 {
		a.mu.RLock()
		target := a.botRuntimeTabLocked(r)
		if target == nil {
			a.mu.RUnlock()
			return errors.New("bot session binding changed")
		}
		tabID := target.ID
		a.mu.RUnlock()
		admission, owner, err := a.beginRuntimeTurnChecked(tabID, false, true, func(owner control.SessionAPI) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if owner != r.ctrl || !a.botBindingValid(r) {
				return errors.New("bot session binding changed")
			}
			if seat := r.ctrl.BoundTeamSeat(); seat != nil {
				_, ref := seat.Team()
				m, err := botTeamMember(ref.Agent)
				if err != nil {
					return err
				}
				if ref.Group != "" || m.SessionPath != sessionRoute(r.config.SessionID) {
					return errors.New("team member binding changed")
				}
			}
			return nil
		}, id)
		if err != nil {
			a.mu.RLock()
			current := a.botRuntimeTabLocked(r)
			moved := current != nil && current.ID != tabID
			a.mu.RUnlock()
			if moved && ctx.Err() == nil {
				continue
			}
			return err
		}
		defer admission.abort()
		if err = a.ensureTabTopicIndexedForUserTurn(admission.tab); err != nil {
			return err
		}
		if _, err = r.ctrl.SubmitIdentifiedContext(ctx, control.SubmissionRequest{ID: id, Input: text, Display: text}); err != nil {
			return err
		}
		admission.finish(owner)
		return nil
	}
	return errors.New("session is moving between tabs; send the message again")
}
func (a *App) forwardBotEvent(r *botChannelRun, e event.Event) {
	if r.config.MemberID != "" && !r.ownsBotEvent(e) {
		return
	}
	switch e.Kind {
	case event.Message:
		r.enqueue(e.Text)
	case event.ApprovalRequest:
		if e.Approval.Kind == "write_access" || e.Approval.Kind == "plan" {
			r.enqueue("A workspace access or plan decision requires review in Desktop.")
			return
		}
		id := e.Approval.ID
		if id == "" || e.TurnID == "" || e.RuntimeEpoch == "" {
			r.enqueue("Approval required. Open Desktop.")
			return
		}
		kind := control.PromptKind(e.PromptKind)
		if kind == "" {
			kind = control.PromptApproval
		}
		if kind != control.PromptApproval {
			r.enqueue("Decision required. Open Desktop.")
			return
		}
		r.mu.Lock()
		r.prompts[id] = control.PromptIdentity{PromptID: id, TurnID: e.TurnID, RuntimeEpoch: e.RuntimeEpoch, Kind: kind}
		if r.config.MemberID != "" {
			r.promptOwners[id] = r.sourceOwners[e.SubmissionID]
		}
		r.mu.Unlock()
		r.enqueue("Approval required: " + e.Approval.Tool + "\n" + e.Approval.Subject + "\n" + e.Approval.Reason + "\n/approve " + id + "\n/deny " + id)
	case event.AskRequest, event.MCPInteractionRequest:
		r.enqueue("A structured question needs your attention in Desktop.")
	case event.PromptAnswered:
		r.mu.Lock()
		delete(r.prompts, e.ItemID)
		delete(r.promptOwners, e.ItemID)
		r.mu.Unlock()
	case event.TurnDone:
		r.mu.Lock()
		for id, p := range r.prompts {
			if r.config.MemberID == "" || p.TurnID == e.TurnID {
				delete(r.prompts, id)
				delete(r.promptOwners, id)
			}
		}
		if r.config.MemberID != "" {
			delete(r.routes, e.SubmissionID)
			delete(r.sourceOwners, e.SubmissionID)
			delete(r.turnRoutes, e.TurnID)
		}
		r.mu.Unlock()
		if e.Err != nil {
			if failure := botModelFailure(r.ctrl); failure != "" {
				r.enqueue(failure)
			} else {
				r.enqueue("Task ended with an error. Open Desktop for details.")
			}
		} else if e.Cancelled {
			r.enqueue("Task cancelled.")
		}
	}
}

// Nonblocking projection of canonical events: network IO never blocks a session.
func (a *App) publishBotEvent(sink *tabEventSink, e event.Event) {
	switch e.Kind {
	case event.TurnStarted, event.Message, event.ApprovalRequest, event.AskRequest, event.MCPInteractionRequest, event.PromptAnswered, event.TurnDone:
	default:
		return
	}
	a.botChannels.mu.Lock()
	defer a.botChannels.mu.Unlock()
	for _, r := range a.botChannels.runs {
		r.mu.Lock()
		matches := r.sink == sink && e.SessionID == r.config.SessionID
		r.mu.Unlock()
		if r.config.MemberID != "" {
			matches = r.ownsBotEvent(e)
		}
		if !matches {
			continue
		}
		select {
		case <-r.done:
			continue
		default:
		}
		select {
		case r.events <- e:
		default:
			r.setStatus("error", "event queue full; reconnect and inspect Desktop")
			r.cancel()
		}
	}
}
func (a *App) stopBotChannels() error {
	a.botChannels.op.Lock()
	defer a.botChannels.op.Unlock()
	a.botChannels.mu.Lock()
	a.botChannels.closed = true
	ids := make([]string, 0, len(a.botChannels.runs))
	for id, r := range a.botChannels.runs {
		ids = append(ids, id)
		r.cancel()
	}
	a.botChannels.mu.Unlock()
	var result error
	for _, id := range ids {
		result = errors.Join(result, a.stopBotChannel(id))
	}
	return result
}
