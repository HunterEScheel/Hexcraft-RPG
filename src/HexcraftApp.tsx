import { Route, Routes } from 'react-router-dom';
import { Layout } from './Layout';
import { Home } from './pages/Home';
import { Builder } from './pages/Builder';
import { Sheet } from './pages/Sheet';
import { RunningTheGame } from './pages/RunningTheGame';
import { MonsterMaker } from './pages/MonsterMaker';

/**
 * Hexcraft's routes, at the root of the site. It once used a hash router; real
 * URLs instead mean its pages are shareable and refreshable.
 */
export function HexcraftApp() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<Home />} />
        <Route path="builder" element={<Builder />} />
        <Route path="builder/:id" element={<Builder />} />
        <Route path="sheet/:id" element={<Sheet />} />
        <Route path="running-the-game" element={<RunningTheGame />} />
        <Route path="monster-maker" element={<MonsterMaker />} />
      </Route>
    </Routes>
  );
}
