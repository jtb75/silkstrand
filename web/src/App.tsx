import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import Layout from './components/Layout';
import Dashboard from './pages/Dashboard';
import Targets from './pages/Targets';
import Scans from './pages/Scans';
import ScanResults from './pages/ScanResults';
import Team from './pages/Team';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route element={<Layout />}>
            <Route path="/" element={<Dashboard />} />
            <Route path="/targets" element={<Targets />} />
            <Route path="/scans" element={<Scans />} />
            <Route path="/scans/:id" element={<ScanResults />} />
            <Route path="/team" element={<Team />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  );
}
