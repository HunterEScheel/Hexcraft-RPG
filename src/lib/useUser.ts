import { useEffect, useState } from 'react';
import type { User } from '@supabase/supabase-js';
import { supabase } from './supabase';

/** Who is signed in, if anyone. Browsing and building need no account; deleting does. */
export function useUser(): User | null {
  const [user, setUser] = useState<User | null>(null);

  useEffect(() => {
    supabase.auth.getSession().then(({ data }) => setUser(data.session?.user ?? null));
    const { data: sub } = supabase.auth.onAuthStateChange((_event, session) => {
      setUser(session?.user ?? null);
    });
    return () => sub.subscription.unsubscribe();
  }, []);

  return user;
}

/**
 * Whether the signed-in account is the site admin (site_admins), who can open
 * every character and see its passcode. Only a UI hint: the database decides.
 */
export function useIsAdmin(user: User | null): boolean {
  const [admin, setAdmin] = useState(false);

  useEffect(() => {
    if (!user) {
      setAdmin(false);
      return;
    }
    let cancelled = false;
    supabase.rpc('is_site_admin').then(({ data }) => {
      if (!cancelled) setAdmin(data === true);
    });
    return () => {
      cancelled = true;
    };
  }, [user]);

  return admin;
}
