import { useEffect } from "react";

declare global {
  interface Window {
    __deepzoraFlushComposerInput?: () => Promise<void>;
    __deepzoraResumeComposerEditing?: () => Promise<void>;
  }
}

type ComposerExitPorts = {
  flushInput: () => Promise<void>; flushTeam: () => Promise<void>;
  resumeInput: () => void; resumeTeam: () => void;
};

// A timed-out shell request can finish after editing resumes or another quit
// begins. Only the current request may freeze or resume the next draft owner.
export function createSessionComposerLifecycle(load: () => Promise<ComposerExitPorts>) {
  let generation = 0;
  return {
    flush: async () => {
      const owner = ++generation;
      const ports = await load();
      const current = () => { if (owner !== generation) throw new Error("Draft save was superseded"); };
      try {
        current();
        await ports.flushInput();
        current();
        await ports.flushTeam();
        current();
      } catch (error) {
        if (owner === generation) { ports.resumeInput(); ports.resumeTeam(); }
        throw error;
      }
    },
    resume: async () => {
      const owner = ++generation;
      const ports = await load();
      if (owner === generation) { ports.resumeInput(); ports.resumeTeam(); }
    },
  };
}

/** The shell's historical callback names now belong only to formal input. */
export function useSessionComposerLifecycle() {
  useEffect(() => {
    const { flush, resume } = createSessionComposerLifecycle(async () => {
      const input = await import("../lib/sessionComposerPersistence");
      const team = await import("../store/teamComposer");
      return { flushInput: input.flushAllSessionComposers, flushTeam: team.flushAllTeamDrafts,
        resumeInput: input.resumeSessionComposerEditing, resumeTeam: team.resumeTeamDraftEditing };
    });
    window.__deepzoraFlushComposerInput = flush;
    window.__deepzoraResumeComposerEditing = resume;
    return () => {
      if (window.__deepzoraFlushComposerInput === flush) delete window.__deepzoraFlushComposerInput;
      if (window.__deepzoraResumeComposerEditing === resume) delete window.__deepzoraResumeComposerEditing;
    };
  }, []);
}
