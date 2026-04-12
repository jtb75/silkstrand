import { StrictMode, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { hasToken } from './api/client';
import TokenPrompt from './components/TokenPrompt';
import App from './App';
import './index.css';

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
