import { useState } from 'react';
import type { ChartCandidate, DeployPlan } from '../../api/types';

interface ChartSelectCardProps {
  appName?: string;
  candidates: ChartCandidate[];
  disabled?: boolean;
  onSelect: (index: number) => void;
  onManual: (repoUrl: string, chartName: string) => void;
  onCancel: () => void;
}

export function ChartSelectCard({
  appName,
  candidates,
  disabled,
  onSelect,
  onManual,
  onCancel,
}: ChartSelectCardProps) {
  const [manual, setManual] = useState(false);
  const [repoUrl, setRepoUrl] = useState('');
  const [chartName, setChartName] = useState('');

  return (
    <div className="mt-3 p-3 rounded-sm border border-accent/25 bg-accent-dim/10 space-y-3">
      <div className="text-xs font-mono text-accent uppercase tracking-wide">Select Helm chart</div>
      {appName && <div className="text-sm text-text-secondary">App: {appName}</div>}

      {!manual ? (
        <>
          <div className="space-y-2 max-h-48 overflow-y-auto">
            {candidates.map((c, i) => (
              <button
                key={`${c.ChartName}-${i}`}
                type="button"
                disabled={disabled}
                onClick={() => onSelect(i)}
                className="w-full text-left p-2 rounded-sm border border-border hover:border-accent/40 bg-bg disabled:opacity-40"
              >
                <div className="text-sm font-medium text-text">
                  {c.ChartName}
                  {c.LatestVersion ? ` @ ${c.LatestVersion}` : ''}
                </div>
                <div className="text-xs text-text-muted font-mono truncate">{c.RepoURL}</div>
                {c.Description && <div className="text-xs text-text-secondary mt-1">{c.Description}</div>}
              </button>
            ))}
          </div>
          <div className="flex gap-2">
            <button
              type="button"
              disabled={disabled}
              onClick={() => setManual(true)}
              className="text-sm px-3 py-1.5 rounded-sm border border-border text-text-secondary"
            >
              Manual chart
            </button>
            <button
              type="button"
              disabled={disabled}
              onClick={onCancel}
              className="text-sm px-3 py-1.5 rounded-sm border border-border text-text-muted"
            >
              Cancel
            </button>
          </div>
        </>
      ) : (
        <div className="space-y-2">
          <input
            value={repoUrl}
            onChange={(e) => setRepoUrl(e.target.value)}
            placeholder="Repo URL"
            className="w-full text-sm bg-bg border border-border rounded-sm px-3 py-2"
          />
          <input
            value={chartName}
            onChange={(e) => setChartName(e.target.value)}
            placeholder="Chart name"
            className="w-full text-sm bg-bg border border-border rounded-sm px-3 py-2"
          />
          <div className="flex gap-2">
            <button
              type="button"
              disabled={disabled || !repoUrl || !chartName}
              onClick={() => onManual(repoUrl.trim(), chartName.trim())}
              className="text-sm px-3 py-1.5 rounded-sm bg-accent text-bg font-medium disabled:opacity-40"
            >
              Use chart
            </button>
            <button type="button" onClick={() => setManual(false)} className="text-sm px-3 py-1.5 rounded-sm border border-border">
              Back
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

interface DeployConfirmCardProps {
  plan: DeployPlan;
  disabled?: boolean;
  onExecute: (values: string) => void;
  onCancel: () => void;
  onCorrect: (correction: string) => void;
}

export function DeployConfirmCard({ plan, disabled, onExecute, onCancel, onCorrect }: DeployConfirmCardProps) {
  const [values, setValues] = useState(plan.CustomValues || '');
  const [correction, setCorrection] = useState('');
  const [mode, setMode] = useState<'review' | 'nl'>('review');

  return (
    <div className="mt-3 p-3 rounded-sm border border-accent/25 bg-accent-dim/10 space-y-3">
      <div className="text-xs font-mono text-accent uppercase tracking-wide">Deploy review</div>
      <div className="text-sm text-text">
        <span className="font-medium">{plan.ChartInfo?.ChartName || 'chart'}</span>
        <span className="text-text-muted"> → {plan.Namespace}/{plan.ReleaseName}</span>
        {plan.IsUpgrade && <span className="ml-2 text-xs text-amber">upgrade</span>}
      </div>

      {plan.Warnings?.map((w, i) => (
        <div key={i} className="text-xs text-amber bg-amber-dim/30 px-2 py-1 rounded-sm">
          {w.Message}
        </div>
      ))}

      {mode === 'review' ? (
        <>
          <textarea
            value={values}
            onChange={(e) => setValues(e.target.value)}
            rows={8}
            className="w-full text-xs font-mono bg-bg border border-border rounded-sm p-2 text-text-secondary"
          />
          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              disabled={disabled}
              onClick={() => onExecute(values)}
              className="text-sm px-3 py-1.5 rounded-sm bg-green text-bg font-medium disabled:opacity-40"
            >
              Deploy
            </button>
            <button
              type="button"
              disabled={disabled}
              onClick={() => setMode('nl')}
              className="text-sm px-3 py-1.5 rounded-sm border border-border text-text-secondary"
            >
              NL correction…
            </button>
            <button
              type="button"
              disabled={disabled}
              onClick={onCancel}
              className="text-sm px-3 py-1.5 rounded-sm border border-border text-text-muted"
            >
              Cancel
            </button>
          </div>
        </>
      ) : (
        <div className="space-y-2">
          <input
            value={correction}
            onChange={(e) => setCorrection(e.target.value)}
            placeholder="e.g. set NodePort to 30090"
            className="w-full text-sm bg-bg border border-border rounded-sm px-3 py-2"
          />
          <div className="flex gap-2">
            <button
              type="button"
              disabled={disabled || !correction.trim()}
              onClick={() => {
                onCorrect(correction.trim());
                setCorrection('');
                setMode('review');
              }}
              className="text-sm px-3 py-1.5 rounded-sm bg-accent text-bg font-medium disabled:opacity-40"
            >
              Send
            </button>
            <button type="button" onClick={() => setMode('review')} className="text-sm px-3 py-1.5 rounded-sm border border-border">
              Back
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
