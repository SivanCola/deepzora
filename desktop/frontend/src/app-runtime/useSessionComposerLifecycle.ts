import { useEffect } from "react";

declare global {
  interface Window {
    __deepzoraFlushComposerInput?: () => Promise<void>;
    __deepzoraResumeComposerEditing?: () => void;
  }
}

/** The shell's historical callback names now belong only to formal input. */
export function useSessionComposerLifecycle() {
  useEffect(() => {
    const flush = async () => {
      const input = await import("../lib/sessionComposerPersistence");
      const team = await import("../store/teamComposer");
      try {
        await input.flushAllSessionComposers();
        await team.flushAllTeamDrafts();
      } catch (error) {
        input.resumeSessionComposerEditing();
        team.resumeTeamDraftEditing();
        throw error;
      }
    };
    const resume = () => {
      void import("../lib/sessionComposerPersistence").then(input => input.resumeSessionComposerEditing());
      void import("../store/teamComposer").then(team => team.resumeTeamDraftEditing());
    };
    window.__deepzoraFlushComposerInput = flush;
    window.__deepzoraResumeComposerEditing = resume;
    return () => {
      if (window.__deepzoraFlushComposerInput === flush) delete window.__deepzoraFlushComposerInput;
      if (window.__deepzoraResumeComposerEditing === resume) delete window.__deepzoraResumeComposerEditing;
    };
  }, []);
}
