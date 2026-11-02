import { Children, cloneElement, isValidElement, useId, type ReactNode } from "react";
import { Tooltip } from "./Tooltip";
import { SettingsSelect } from "./SettingsSelect";

export function SettingsSection({
  title,
  className,
  description,
  actions,
  children,
}: {
  title?: ReactNode;
  className?: string;
  description?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
}) {
  const hasHead = Boolean(title || description || actions);
  return (
    <section className={`settings-section${className ? ` ${className}` : ""}`}>
      {hasHead && (
        <div className="settings-section__head">
          <div>
            {title && <div className="settings-section__title">{isValidElement(title) ? title : <h2>{title}</h2>}</div>}
            {description && (
              <div className="settings-section__desc">
                <SettingsHint hint={description} />
              </div>
            )}
          </div>
          {actions && <div className="settings-section__actions">{actions}</div>}
        </div>
      )}
      <div className="settings-section__body">{children}</div>
    </section>
  );
}

export function SettingsField({
  label,
  hint,
  icon,
  children,
  className,
  stacked = false,
}: {
  label: ReactNode;
  hint?: ReactNode;
  icon?: ReactNode;
  children: ReactNode;
  className?: string;
  stacked?: boolean;
}) {
  const id = useId();
  const labelId = `${id}-label`;
  const hintId = hint ? `${id}-hint` : undefined;
  return (
    <div className={`settings-field${stacked ? " settings-field--stacked" : ""}${className ? ` ${className}` : ""}`}>
      <div className={`settings-field__copy${icon ? " settings-field__copy--icon" : ""}`}>
        {icon && <span className="settings-field__icon" aria-hidden="true">{icon}</span>}
        <div className="settings-field__copy-body">
          <div id={labelId} className="settings-field__label">{label}</div>
          {hint && (
            <div id={hintId} className="settings-field__hint">
              <SettingsHint hint={hint} />
            </div>
          )}
        </div>
      </div>
      <div className="settings-field__control">{Children.map(children, child => {
        if (!isValidElement<Record<string, unknown>>(child)) return child;
        if (child.type !== "input" && child.type !== "select" && child.type !== "textarea" && child.type !== SettingsSelect) return child;
        return cloneElement(child, {
          "aria-labelledby": child.props["aria-label"] ? child.props["aria-labelledby"] : child.props["aria-labelledby"] ?? labelId,
          "aria-describedby": child.props["aria-describedby"] ?? hintId,
        });
      })}</div>
    </div>
  );
}

function SettingsHint({ hint }: { hint: ReactNode }) {
  if (typeof hint === "string" || typeof hint === "number") {
    const label = String(hint);
    return (
      <Tooltip label={label} fill block className="settings-field__hint-tooltip">
        <span className="settings-field__hint-line">{label}</span>
      </Tooltip>
    );
  }
  return hint;
}
