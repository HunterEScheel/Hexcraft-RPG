import { createBrowserClient } from '@supabase/ssr';

const url = import.meta.env.VITE_SUPABASE_URL as string;
const anonKey = import.meta.env.VITE_SUPABASE_ANON_KEY as string;

if (!url || !anonKey) {
  throw new Error('Missing VITE_SUPABASE_URL / VITE_SUPABASE_ANON_KEY — see .env.example');
}

// Every app on *.jaeg.click shares one sign-in. The session lives in a cookie on
// the parent domain rather than in this origin's localStorage, so signing in on
// any subdomain signs you in on all of them. Locally there is no parent domain to
// share, and the cookie stays on the host.
const hostname = typeof location === 'undefined' ? '' : location.hostname;
const onJaegClick = hostname === 'jaeg.click' || hostname.endsWith('.jaeg.click');

export const supabase = createBrowserClient(url, anonKey, {
  cookieOptions: onJaegClick
    ? { domain: '.jaeg.click', path: '/', sameSite: 'lax', secure: true, maxAge: 400 * 24 * 60 * 60 }
    : undefined,
});

/**
 * The Hexcraft code was written against a possibly-absent client plus this flag.
 * The client above throws on a missing key rather than returning null, so the flag
 * is always true; the checks it guards (custom skills still work without a
 * database) are kept in case that ever changes.
 */
export const supabaseConfigured = true;
