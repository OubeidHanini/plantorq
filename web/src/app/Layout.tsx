import { NavLink, Outlet } from "react-router-dom";
import { useAuth, useUser } from "@/features/auth/AuthContext";
import { useLiveSync } from "./LiveSync";

const NAV = [
  { to: "/", label: "Overview", end: true },
  { to: "/alerts", label: "Alerts" },
  { to: "/rules", label: "Alert rules" },
];

const connectionLabel = { open: "Live", connecting: "Connecting…", closed: "Reconnecting…" } as const;

export function Layout() {
  const user = useUser();
  const { logout } = useAuth();
  const connection = useLiveSync();

  return (
    <div className="shell">
      <nav className="sidebar" aria-label="Main">
        <div className="brand">
          <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true">
            <path d="M3 21V10l5 3V10l5 3V6l8 4v11z" />
            <path d="M3 21h18" />
          </svg>
          Plantorq
        </div>
        <div className="tenant">
          <span className="muted small">Tenant</span>
          <strong>{user.tenant}</strong>
          <span className={`connection connection--${connection}`} role="status">
            <span className="connection__dot" />
            {connectionLabel[connection]}
          </span>
        </div>
        <ul className="nav">
          {NAV.map((n) => (
            <li key={n.to}>
              <NavLink to={n.to} end={n.end} className={({ isActive }) => `nav__link ${isActive ? "is-active" : ""}`}>
                {n.label}
              </NavLink>
            </li>
          ))}
        </ul>
        <div className="user">
          <strong>{user.name || user.email}</strong>
          <span className="muted small">{user.role}</span>
          <button className="button" onClick={logout}>
            Sign out
          </button>
        </div>
      </nav>
      <main className="content">
        <Outlet />
      </main>
    </div>
  );
}
