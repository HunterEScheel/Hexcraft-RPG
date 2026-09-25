import { useState } from 'react';
import { Navigate } from 'react-router-dom';
import { supabase } from '../lib/supabase';
import { useUser } from '../lib/useUser';

/**
 * Optional: Hexcraft is open to everyone, and an account is only needed to
 * delete characters. Sign-in returns to the roster.
 */
function redirectTo() {
  return window.location.origin + '/';
}

export function SignIn() {
  const user = useUser();
  const [email, setEmail] = useState('');
  const [sent, setSent] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function oauth(provider: 'github' | 'discord') {
    setError(null);
    const { error } = await supabase.auth.signInWithOAuth({
      provider,
      options: { redirectTo: redirectTo() },
    });
    if (error) setError(error.message);
  }

  async function sendLink(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    const { error } = await supabase.auth.signInWithOtp({
      email,
      options: { emailRedirectTo: redirectTo() },
    });
    if (error) setError(error.message);
    else setSent(true);
  }

  if (user) return <Navigate to="/" replace />;

  return (
    <div className="flex justify-center py-8">
      <form onSubmit={sendLink} className="w-80 space-y-4 rounded-xl bg-zinc-900 p-6">
        <h1 className="text-lg font-semibold">Sign in</h1>
        <p className="text-sm text-zinc-400">Only needed to delete characters.</p>
        <div className="space-y-2">
          {([
            ['github', 'Continue with GitHub'],
            ['discord', 'Continue with Discord'],
          ] as const).map(([provider, label]) => (
            <button
              key={provider}
              type="button"
              onClick={() => oauth(provider)}
              className="w-full rounded-md bg-zinc-800 px-3 py-2 text-sm font-medium ring-1 ring-zinc-700 hover:bg-zinc-700"
            >
              {label}
            </button>
          ))}
        </div>
        <div className="flex items-center gap-2 text-xs text-zinc-500">
          <span className="h-px flex-1 bg-zinc-700" /> or email link <span className="h-px flex-1 bg-zinc-700" />
        </div>
        {sent ? (
          <p className="text-sm text-emerald-400">
            Check your email for a sign-in link.
          </p>
        ) : (
          <>
            <input
              type="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="you@example.com"
              className="w-full rounded-md bg-zinc-800 px-3 py-2 text-sm outline-none ring-1 ring-zinc-700 focus:ring-indigo-500"
            />
            <button
              type="submit"
              className="w-full rounded-md bg-indigo-600 px-3 py-2 text-sm font-medium hover:bg-indigo-500"
            >
              Send magic link
            </button>
            {error && <p className="text-sm text-red-400">{error}</p>}
          </>
        )}
      </form>
    </div>
  );
}
