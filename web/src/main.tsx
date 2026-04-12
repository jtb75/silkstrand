import { StrictMode, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { hasToken } from './api/client';
import TokenPrompt from './components/TokenPrompt';
import App from './App';
import './index.css';

const clerkKey = import.meta.env.VITE_CLERK_PUBLISHABLE_KEY as string | undefined;

if (clerkKey) {
  // Clerk mode — dynamically import to avoid bundling Clerk in dev builds
  import('@clerk/clerk-react').then(({ ClerkProvider }) => {
    import('./components/ClerkAuthProvider').then(({ default: ClerkAuthProvider }) => {
      createRoot(document.getElementById('root')!).render(
        <StrictMode>
          <ClerkProvider publishableKey={clerkKey}>
            <ClerkAuthProvider>
              <App />
            </ClerkAuthProvider>
          </ClerkProvider>
        </StrictMode>,
      );
    });
  });
} else {
  // Dev mode — existing token prompt flow
  function Root() {
    const [authenticated, setAuthenticated] = useState(hasToken());

    if (!authenticated) {
      return <TokenPrompt onTokenSet={() => setAuthenticated(true)} />;
    }

    return <App />;
  }

  createRoot(document.getElementById('root')!).render(
    <StrictMode>
      <Root />
    </StrictMode>,
  );
}
