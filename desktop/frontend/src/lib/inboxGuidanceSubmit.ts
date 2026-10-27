import type { AppBindings } from "./bridge";
import { resolveActiveTurnId } from "./inboxSubmit";
import type { PendingFollowup } from "./pendingFollowup";

export async function enqueueComposerGuidance(binding: AppBindings, request: PendingFollowup, queueOnly: boolean, turnId?: string) {
  const { target, structured, tabId, display, submit, key } = request;
  if (structured?.attachments?.length) {
    if (!structured.attachmentTarget || !binding.EnqueueForAttachmentTarget) throw new Error("unsupported: attachments-v2");
    return binding.EnqueueForAttachmentTarget(
      structured.attachmentTarget,
      structured.attachmentSubmissionId || key,
      structured.input.trim(),
      structured.display.trim() || display,
      structured.invocations,
      structured.attachments,
    );
  }
  if (!target) throw new Error("deepzora_error:inbox_not_submitted — target submission unavailable");
  if (queueOnly || structured) {
    return binding.EnqueueInboxFollowupForTarget(target, display, structured?.input ?? submit, structured?.invocations ?? [], key);
  }
  const followup = () => binding.EnqueueInboxFollowupForTarget(target, display, submit, [], key);
  const activeTurnId = await resolveActiveTurnId(binding, tabId, turnId).catch(() => undefined);
  if (!activeTurnId) return followup();
  const result = await binding.InboxQueueForTarget(target, { kind: "enqueue_steer", text: submit, display, turnId: activeTurnId, idempotencyKey: key });
  if (result.outcome === "unavailable" && result.reason === "session_changed") {
    // Remote selection can change after the POST committed. Retain the
    // pending request for receipt recovery instead of declaring it unsent.
    throw new Error("Follow-up receipt unconfirmed — session_changed");
  }
  if (result.receipt?.error) throw new Error(result.receipt.error);
  if (!result.receipt?.itemId) throw new Error("Follow-up receipt unconfirmed");
  return result.receipt;
}
