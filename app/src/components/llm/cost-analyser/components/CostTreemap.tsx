/**
 * CostTreemap — cost composition within a run (spec §6d).
 * Segmented horizontal bar (a 1-D treemap): which steps / models drove spend.
 * Box + DS tokens, no charting dependency.
 */
import * as React from 'react';
import { Box } from '@mui/material';
import Tooltip from '@ui/Tooltip';
import { fmtCost, fmtPct } from '../format';

export interface TreemapSlice {
  key: string;
  cost: number;
}

interface CostTreemapProps {
  slices: TreemapSlice[];
  /** Total to compute share against; defaults to sum of slices. */
  total?: number;
}

const PALETTE = [
  'var(--ds-blue-400)',
  'var(--ds-purple-400)',
  'var(--ds-teal-400)',
  'var(--ds-amber-400)',
  'var(--ds-pink-400)',
  'var(--ds-green-400)',
  'var(--ds-red-400)',
  'var(--ds-gray-400)',
];

export function CostTreemap({ slices, total }: CostTreemapProps) {
  const sorted = [...slices].sort((a, b) => b.cost - a.cost);
  const sum = total ?? (sorted.reduce((a, s) => a + s.cost, 0) || 1);

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 'var(--ds-space-2)' }} id='cost-treemap'>
      <Box
        sx={{
          display: 'flex',
          width: '100%',
          height: 28,
          borderRadius: 'var(--ds-radius-sm)',
          overflow: 'hidden',
          border: '1px solid var(--ds-gray-200)',
        }}
      >
        {sorted.map((s, i) => {
          const pct = (s.cost / sum) * 100;
          if (pct <= 0) return null;
          return (
            <Tooltip key={s.key} title={`${s.key} · ${fmtCost(s.cost)} · ${fmtPct(s.cost / sum)}`} placement='top'>
              <Box
                sx={{
                  width: `${pct}%`,
                  height: '100%',
                  backgroundColor: PALETTE[i % PALETTE.length],
                  borderRight: i < sorted.length - 1 ? '1px solid var(--ds-background-100)' : 'none',
                }}
              />
            </Tooltip>
          );
        })}
      </Box>
      <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 'var(--ds-space-2) var(--ds-space-4)' }}>
        {sorted.map((s, i) => (
          <Box
            key={s.key}
            sx={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: 'var(--ds-space-1)',
              fontSize: 'var(--ds-text-caption)',
              color: 'var(--ds-gray-700)',
            }}
          >
            <Box sx={{ width: 10, height: 10, borderRadius: 2, backgroundColor: PALETTE[i % PALETTE.length], flexShrink: 0 }} />
            <Box component='span'>{s.key}</Box>
            <Box component='span' sx={{ color: 'var(--ds-gray-500)', fontVariantNumeric: 'tabular-nums' }}>
              {fmtCost(s.cost)} · {fmtPct(s.cost / sum)}
            </Box>
          </Box>
        ))}
      </Box>
    </Box>
  );
}

export default CostTreemap;
