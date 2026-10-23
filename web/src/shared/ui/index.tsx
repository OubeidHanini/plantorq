import type { ReactNode } from "react";

// Small presentational primitives. They know nothing about the domain, so
// every feature composes the same visual language.

export function Card({ title, action, children, className = "" }: { title?: string; action?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={`card ${className}`}>
      {(title || action) && (
        <header className="card__header">
          {title && <h2 className="card__title">{title}</h2>}
          {action}
        </header>
      )}
      {children}
    </section>
  );
}

export type Tone = "info" | "ok" | "warn" | "crit" | "muted";

export function Badge({ tone, children }: { tone: Tone; children: ReactNode }) {
  return <span className={`badge badge--${tone}`}>{children}</span>;
}

export function Stat({ label, value, unit, hint, tone }: { label: string; value: ReactNode; unit?: string; hint?: ReactNode; tone?: Tone }) {
  return (
    <div className="card stat">
      <span className="stat__label">{label}</span>
      <span className={`stat__value ${tone ? `text--${tone}` : ""}`}>
        {value}
        {unit && <span className="stat__unit"> {unit}</span>}
      </span>
      {hint && <span className="stat__hint">{hint}</span>}
    </div>
  );
}

export function Empty({ children }: { children: ReactNode }) {
  return <p className="empty">{children}</p>;
}

export const fmt = (v: number | undefined, digits = 1) => (v === undefined ? "—" : v.toFixed(digits));

export function timeAgo(iso: string | number): string {
  const s = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000));
  if (s < 60) return `${s}s ago`;
  if (s < 3600) return `${Math.round(s / 60)} min ago`;
  return `${Math.round(s / 3600)} h ago`;
}
