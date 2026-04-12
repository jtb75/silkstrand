import { useEffect, type ReactNode } from 'react';
import {
  SignedIn,
  SignedOut,
  SignIn,
  useAuth,
} from '@clerk/clerk-react';
import { setClerkTokenGetter, clearClerkTokenGetter } from '../api/client';

/**
 * Bridge between Clerk auth and the API client.
 * Registers a token getter so API requests automatically include the Clerk session token.
 */
function ClerkTokenBridge({ children }: { children: ReactNode }) {
  const { getToken, isSignedIn } = useAuth();

  useEffect(() => {
    if (isSignedIn) {
      setClerkTokenGetter(() => getToken());
    }
    return () => {
      clearClerkTokenGetter();
    };
  }, [isSignedIn, getToken]);

  return <>{children}</>;
}

/**
 * Wraps children with Clerk authentication.
 * Shows the Clerk sign-in form when unauthenticated.
 * Injects the Clerk session token into the API client when authenticated.
 */
export default function ClerkAuthProvider({ children }: { children: ReactNode }) {
  return (
    <>
      <SignedOut>
        <div style={{ display: 'flex', justifyContent: 'center', marginTop: '4rem' }}>
          <SignIn />
        </div>
      </SignedOut>
      <SignedIn>
        <ClerkTokenBridge>
          {children}
        </ClerkTokenBridge>
      </SignedIn>
    </>
  );
}
