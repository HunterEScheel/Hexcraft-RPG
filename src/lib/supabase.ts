import { createClient } from '@supabase/supabase-js';

const url = import.meta.env.VITE_SUPABASE_URL as string;
const anonKey = import.meta.env.VITE_SUPABASE_ANON_KEY as string;

if (!url || !anonKey) {
  throw new Error('Missing VITE_SUPABASE_URL / VITE_SUPABASE_ANON_KEY — see .env.example');
}

export const supabase = createClient(url, anonKey);

/**
 * The Hexcraft code was written against a possibly-absent client plus this flag.
 * The client above throws on a missing key rather than returning null, so the flag
 * is always true; the checks it guards (custom skills still work without a
 * database) are kept in case that ever changes.
 */
export const supabaseConfigured = true;
