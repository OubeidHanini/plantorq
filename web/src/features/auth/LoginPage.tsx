import { useState, type FormEvent } from "react";
import { ApiError } from "@/shared/api/client";
import { useAuth } from "./AuthContext";

export function LoginPage() {
  const { login } = useAuth();
  const [email, setEmail] = useState("admin@acme.io");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await login(email, password);
    } catch (err) {
      setError(err instanceof ApiError && err.status === 401 ? "Wrong e-mail or password." : "Server unreachable, try again.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="login">
      <form className="card login__form" onSubmit={submit}>
        <h1 className="login__title">Plantorq</h1>
        <p className="muted">Industrial monitoring platform</p>
        <label className="field">
          E-mail
          <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="username" required />
        </label>
        <label className="field">
          Password
          <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" required />
        </label>
        {error && <p className="text--crit" role="alert">{error}</p>}
        <button className="button button--primary" disabled={busy}>
          {busy ? "Signing in…" : "Sign in"}
        </button>
        <p className="muted small">Demo accounts: admin@acme.io · operator@acme.io · viewer@acme.io</p>
      </form>
    </main>
  );
}
