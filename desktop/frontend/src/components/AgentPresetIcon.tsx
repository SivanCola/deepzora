import { BookOpen, Bot, Brain, Bug, Code, Database, FlaskConical, Globe, GraduationCap, Palette, PenLine, Rocket, Search, ShieldCheck, Sparkles, SquareTerminal } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { useState } from "react";
import { isPresetIconImage } from "../lib/agentPresetImage";

export const DEFAULT_AGENT_PRESET_ICON = "bot";

// Keys are persisted in preset files, so renaming one orphans saved presets
// (they fall back to the default glyph). Add keys; never rename them.
export const AGENT_PRESET_ICONS: Record<string, LucideIcon> = {
  bot: Bot,
  code: Code,
  terminal: SquareTerminal,
  "book-open": BookOpen,
  pen: PenLine,
  search: Search,
  bug: Bug,
  flask: FlaskConical,
  shield: ShieldCheck,
  palette: Palette,
  rocket: Rocket,
  brain: Brain,
  sparkles: Sparkles,
  database: Database,
  globe: Globe,
  school: GraduationCap,
};

export function AgentPresetIcon({ icon, image, color, size = 16 }: { icon?: string; image?: string; color?: string; size?: number }) {
  const [failedImage, setFailedImage] = useState<string>();
  const Glyph = icon && Object.prototype.hasOwnProperty.call(AGENT_PRESET_ICONS, icon) ? AGENT_PRESET_ICONS[icon] : Bot;
  if (image && image !== failedImage && isPresetIconImage(image)) return <img className="agent-preset-icon-image" src={image}
    width={size} height={size} alt="" aria-hidden="true" draggable={false} onError={() => setFailedImage(image)} />;
  return <Glyph size={size} aria-hidden="true" style={color && /^#[0-9a-f]{6}$/iu.test(color) ? { color } : undefined} />;
}
