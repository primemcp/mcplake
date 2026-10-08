import { createContext, useContext } from "react";

/**
 * The authentication context itself, split out from AuthProvider so that
 * file exports only its component (React Fast Refresh requires it, and
 * consumers that just need `useAuth` don't pull in the provider).
 */

export type Auth = {
  /** Who the operator is signed in as, as the token's most readable
   * identity claim. Null when the token carries none. */
  subject: string | null;
  signOut: () => void;
};

/** Null means admin auth is off: there is nobody to be signed in as. */
export const AuthContext = createContext<Auth | null>(null);

/** Identity and sign-out for the app chrome. Returns null when admin auth
 * is off, so the sidebar can simply render nothing. */
export function useAuth(): Auth | null {
  return useContext(AuthContext);
}
