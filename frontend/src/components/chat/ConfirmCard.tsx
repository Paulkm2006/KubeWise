import { useState } from 'react';
import type { OperationStep } from '../../api/types';

interface ConfirmCardProps {
  step: OperationStep;
  totalSteps?: number;
  disabled?: boolean;
  onConfirm: () => void;
  onSkip: () => void;
  onCorrect: (correction: string) => void;
}

function opLabel(op: string): string {
  const map: Record<string, string> = {
    scale: 'Scale',
    restart: 'Restart',
    delete: 'Delete',
    apply: 'Apply YAML',
    cordon_drain: 'Cordon / Drain',
    label_annotate: 'Label / Annotate',
  };
  return map[op] || op;
}

export default function ConfirmCard({
  step,
  totalSteps,
  disabled,
  onConfirm,
  onSkip,
  onCorrect,
}: ConfirmCardProps) {
  const [mode, setMode] = useState<'choice' | 'edit'>('choice');
  const [correction, setCorrection] = useState('');

  const idx = step.step_index ?? 1;
  const total = totalSteps || idx;

  return (
    <div className="mt-3 p-3 rounded-sm border border-amber/30 bg-amber-dim/20 space-y-3">
      <div className="text-xs font-mono text-amber uppercase tracking-wide">Confirm operation</div>
      <div className="text-sm text-text">
        <span className="text-text-muted">Step {idx}/{total} · </span>
        <span className="font-medium">{opLabel(step.operation_type)}</span>
      </div>
      <div className="text-sm text-text-secondary font-mono">
        {step.namespace ? `${step.namespace}/` : ''}
        {step.resource_kind}/{step.resource_name}
      </div>
      {step.replicas != null && (
        <div className="text-sm text-text-secondary">replicas → {step.replicas}</div>
      )}
      {step.generated_yaml && (
        <pre className="text-xs bg-bg border border-border rounded-sm p-2 overflow-x-auto max-h-40 text-text-secondary">
          {step.generated_yaml}
        </pre>
      )}
      {step.description && <p className="text-sm text-text-secondary">{step.description}</p>}

      {mode === 'choice' ? (
        <div className="flex flex-wrap gap-2">
          <button
            type="button"
            disabled={disabled}
            onClick={onConfirm}
            className="text-sm px-3 py-1.5 rounded-sm bg-green text-bg font-medium disabled:opacity-40"
          >
            Confirm
          </button>
          <button
            type="button"
            disabled={disabled}
            onClick={onSkip}
            className="text-sm px-3 py-1.5 rounded-sm border border-border text-text-secondary hover:border-red/40"
          >
            Skip
          </button>
          <button
            type="button"
            disabled={disabled}
            onClick={() => setMode('edit')}
            className="text-sm px-3 py-1.5 rounded-sm border border-border text-text-secondary hover:border-accent/30"
          >
            Correct…
          </button>
        </div>
      ) : (
        <div className="space-y-2">
          <input
            value={correction}
            onChange={(e) => setCorrection(e.target.value)}
            placeholder="Describe your correction…"
            className="w-full text-sm bg-bg border border-border rounded-sm px-3 py-2 outline-none focus:border-accent/30"
          />
          <div className="flex gap-2">
            <button
              type="button"
              disabled={disabled}
              onClick={() => {
                onCorrect(correction.trim());
                setCorrection('');
                setMode('choice');
              }}
              className="text-sm px-3 py-1.5 rounded-sm bg-accent text-bg font-medium disabled:opacity-40"
            >
              Send correction
            </button>
            <button
              type="button"
              onClick={() => {
                setMode('choice');
                setCorrection('');
              }}
              className="text-sm px-3 py-1.5 rounded-sm border border-border text-text-muted"
            >
              Cancel
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
