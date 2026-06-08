import { useState } from 'react';
import { auditFindings } from '../data/mock';

const sevStyle: Record<string, string> = {
  high: 'text-red bg-red-dim',
  medium: 'text-amber bg-amber-dim',
  low: 'text-accent bg-accent-dim',
};

const FILTERS = ['All', 'HIGH', 'MEDIUM', 'LOW'] as const;

export default function SecurityAudit() {
  const [filter, setFilter] = useState('All');

  const filtered = filter === 'All'
    ? auditFindings
    : auditFindings.filter((f) => f.severity.toUpperCase() === filter);

  const counts = {
    total: auditFindings.length,
    high: auditFindings.filter((f) => f.severity === 'high').length,
    medium: auditFindings.filter((f) => f.severity === 'medium').length,
    low: auditFindings.filter((f) => f.severity === 'low').length,
  };

  return (
    <div className="h-full overflow-y-auto px-8 py-6">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-sm font-semibold text-text tracking-wide">Security Audit Report</h1>
        <button className="text-sm text-accent px-4 py-1.5 border border-accent/30 rounded-sm
                           hover:bg-accent-dim/15 transition-colors cursor-pointer bg-transparent font-medium">
          ↓ Export
        </button>
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-4 gap-4 mb-6">
        {[
          { label: 'Total', value: counts.total, color: 'text-text' },
          { label: 'HIGH', value: counts.high, color: 'text-red' },
          { label: 'MEDIUM', value: counts.medium, color: 'text-amber' },
          { label: 'LOW', value: counts.low, color: 'text-accent' },
        ].map((c) => (
          <button
            key={c.label}
            onClick={() => setFilter(c.label === 'Total' ? 'All' : c.label)}
            className={`border rounded-sm p-5 text-center transition-all duration-150 cursor-pointer
              ${filter === (c.label === 'Total' ? 'All' : c.label)
                ? 'border-accent/40 bg-accent-dim/10'
                : 'border-border bg-surface hover:bg-elevated'
              }`}
          >
            <p className={`text-2xl font-semibold font-mono ${c.color}`}>{c.value}</p>
            <p className="text-sm text-text-muted mt-1">{c.label}</p>
          </button>
        ))}
      </div>

      {/* Filter bar */}
      <div className="flex gap-2 mb-5">
        {FILTERS.map((f) => (
          <button
            key={f}
            onClick={() => setFilter(f)}
            className={`text-sm px-4 py-1.5 rounded-sm border transition-all duration-150 cursor-pointer font-medium
              ${filter === f
                ? 'border-accent/40 text-accent bg-accent-dim/15'
                : 'border-border text-text-muted hover:border-accent/30 hover:text-text bg-transparent'
              }`}
          >
            {f}
          </button>
        ))}
      </div>

      {/* Table */}
      <div className="border border-border rounded-sm overflow-hidden">
        <table className="w-full text-sm">
          <thead>
            <tr className="bg-elevated/50">
              <th className="text-left text-xs text-text-muted font-semibold uppercase py-3 px-4">Sev</th>
              <th className="text-left text-xs text-text-muted font-semibold uppercase py-3 px-4">Category</th>
              <th className="text-left text-xs text-text-muted font-semibold uppercase py-3 px-4">Resource</th>
              <th className="text-left text-xs text-text-muted font-semibold uppercase py-3 px-4">Risk</th>
              <th className="text-left text-xs text-text-muted font-semibold uppercase py-3 px-4">Impact</th>
              <th className="text-left text-xs text-text-muted font-semibold uppercase py-3 px-4">Suggestion</th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((row, i) => (
              <tr key={i} className="border-t border-border/30 hover:bg-hover/30 transition-colors animate-fade-in">
                <td className="py-3.5 px-4">
                  <span className={`text-xs font-semibold px-2 py-0.5 rounded-sm ${sevStyle[row.severity]}`}>
                    {row.severity.toUpperCase()}
                  </span>
                </td>
                <td className="py-3.5 px-4 text-text-secondary">{row.category}</td>
                <td className="py-3.5 px-4 font-mono text-text-muted text-sm">{row.resource}</td>
                <td className="py-3.5 px-4 font-mono text-text-muted text-sm">{row.risk}</td>
                <td className="py-3.5 px-4 text-text-muted text-sm">{row.impact}</td>
                <td className="py-3.5 px-4 text-accent text-sm font-medium">{row.suggestion}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {filtered.length === 0 && (
        <div className="text-center py-12 text-sm text-text-muted">
          No {filter.toLowerCase()} findings
        </div>
      )}

      <div className="mt-4 text-xs text-text-muted">
        CIS Kubernetes Benchmark v1.10 · NIST SP 800-204 · 2026-06-07 08:15 UTC
      </div>
    </div>
  );
}
