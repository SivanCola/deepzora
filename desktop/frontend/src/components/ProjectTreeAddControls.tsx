import type { MouseEvent as ReactMouseEvent } from "react";
import { FolderPlus } from "lucide-react";

import { ContextMenu, type ContextMenuItem, type ContextMenuPoint } from "./ContextMenu";
import { Tooltip } from "./Tooltip";

export function projectTreeHeaderAddItems({
  blankLabel,
  localLabel,
  disabled,
  onBlank,
  onLocal,
}: {
  blankLabel?: string;
  localLabel: string;
  disabled: boolean;
  onBlank?: () => void;
  onLocal: () => void;
}): ContextMenuItem[] {
  const items: ContextMenuItem[] = [];
  if (onBlank && blankLabel) {
    items.push({ key: "blank-project", icon: <FolderPlus size={13} />, label: blankLabel, disabled, onSelect: onBlank });
  }
  items.push({ key: "open-local-folder", icon: <FolderPlus size={13} />, label: localLabel, disabled, onSelect: onLocal });
  return items;
}

export function ProjectTreeHeaderAddControl({
  open,
  point,
  items,
  label,
  disabled,
  onOpen,
  onClose,
}: {
  open: boolean;
  point: ContextMenuPoint | null;
  items: ContextMenuItem[];
  label: string;
  disabled: boolean;
  onOpen: (event: ReactMouseEvent<HTMLButtonElement>) => void;
  onClose: () => void;
}) {
  return (
    <span className="project-tree__header-menu-wrap">
      <Tooltip label={label} className="project-tree__action-slot project-tree__header-action-slot project-tree__action-slot--add">
        <button
          type="button"
          className={`project-tree__add-project${open ? " project-tree__header-icon-btn--active" : ""}`}
          aria-label={label}
          aria-haspopup="menu"
          aria-expanded={open}
          disabled={disabled}
          onClick={onOpen}
        >
          <FolderPlus size={14} />
        </button>
      </Tooltip>
      <ContextMenu open={open} point={point} items={items} minWidth={206} ariaLabel={label} onClose={onClose} />
    </span>
  );
}
