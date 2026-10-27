import { useEffect } from "react";
import {
  onEvent,
  onReady,
  onRuntimeRebuilt,
} from "../lib/bridge";
import { generativeMusic, isGenerativeMusicEnabled } from "../lib/generative-music";
import { startTerminalEventBridge } from "../lib/terminalEvents";
import { trackAppSubscription } from "./appLifecycleProbe";
import { createSubscriptionScope } from "../lib/subscriptionScope";

export type RuntimeEventListener = Parameters<typeof onEvent>[0];
export type RuntimeReadyListener = Parameters<typeof onReady>[0];
export type RuntimeRebuiltListener = Parameters<typeof onRuntimeRebuilt>[0];

type AppRuntimeEffectsProps = {
  running: boolean;
  onEvent: RuntimeEventListener;
  onReady: RuntimeReadyListener;
  onRebuilt: RuntimeRebuiltListener;
};

/** Owns app-wide bridge subscriptions; App regions never subscribe directly. */
export function AppRuntimeEffects(props: AppRuntimeEffectsProps) {
  const { onEvent: eventListener, onReady: readyListener, onRebuilt, running } = props;
  useEffect(startTerminalEventBridge, []);
  useEffect(() => {
    const scope = createSubscriptionScope(trackAppSubscription);
    scope.listen(onEvent, (event) => {
        eventListener(event);
        if (event.kind === "text" || event.kind === "reasoning" || event.kind === "tool_dispatch") {
          generativeMusic.playTokenNote();
        }
    });
    scope.listen(onReady, readyListener);
    scope.listen(onRuntimeRebuilt, onRebuilt);
    return () => scope.dispose();
  }, [eventListener, readyListener, onRebuilt]);

  useEffect(() => {
    if (running && isGenerativeMusicEnabled()) generativeMusic.start();
    else generativeMusic.stop();
    return () => generativeMusic.stop();
  }, [running]);

  return null;
}
