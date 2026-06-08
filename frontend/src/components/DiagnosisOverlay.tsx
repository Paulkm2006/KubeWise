import { useState, useEffect, useRef } from 'react';
import { diagnosisSteps, mockDiagnosisResult, DiagnosisResult } from '../data/mock';

interface DiagnosisOverlayProps {
  open: boolean;
  cluster: string;
  namespace: string;
  pod: string;
  onClose: () => void;
  onActivity: (type: string, text: string, cluster?: string) => void;
}

interface CachedEntry {
  result: DiagnosisResult;
  ts: number;
}

export default function DiagnosisOverlay({ open, cluster, namespace, pod, onClose, onActivity }: DiagnosisOverlayProps) {
  const [phase, setPhase] = useState<'idle' | 'running' | 'done'>('idle');
  const [currentStep, setCurrentStep] = useState(0);
  const [result, setResult] = useState<DiagnosisResult | null>(null);
  const [copied, setCopied] = useState(false);
  const cacheRef = useRef<Map<string, CachedEntry>>(new Map());
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const cacheKey = `${cluster}/${namespace}/${pod}`;

  useEffect(() => {
    if (open) {
      // Check cache
      const cached = cacheRef.current.get(cacheKey);
      if (cached && Date.now() - cached.ts < 5 * 60 * 1000) {
        setResult(cached.result);
        setPhase('done');
        return;
      }
      startDiagnosis();
    } else {
      // Reset on close
      setPhase('idle');
      setCurrentStep(0);
      setResult(null);
      if (timerRef.current) clearTimeout(timerRef.current);
    }
  }, [open]);

  const startDiagnosis = () => {
    setPhase('running');
    setCurrentStep(0);
    setResult(null);
    onActivity('pending', `Diagnosing ${pod}...`, cluster);

    // Simulate step progression
    const delays = [1200, 2800, 4500, 6000];
    diagnosisSteps.forEach((_, i) => {
      timerRef.current = setTimeout(() => {
        setCurrentStep(i + 1);
        if (i === diagnosisSteps.length - 1) {
          // Done — show report
          setTimeout(() => {
            setPhase('done');
            setResult(mockDiagnosisResult);
            cacheRef.current.set(cacheKey, { result: mockDiagnosisResult, ts: Date.now() });
            onActivity('done', `${pod} diagnosis: OOMKilled`, cluster);
          }, 800);
        }
      }, delays[i]);
    });
  };

  const handleRediagnose = () => {
    cacheRef.current.delete(cacheKey);
    startDiagnosis();
  };

  const handleCopy = async () => {
    if (!result) return;
    try {
      await navigator.clipboard.writeText(result.fixCommand);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch { /* fallback */ }
  };

  if (!open) return null;

  return (
    <>
      {/* Backdrop */}
      <div className="fixed inset-0 z-40 bg-black/40" onClick={onClose} />

      {/* Panel */}
      <div className="fixed top-0 right-0 bottom-0 w-[520px] z-50 bg-surface border-l border-border shadow-2xl
                      overflow-y-auto animate-slide-in">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-border">
          <div>
            <h2 className="text-sm font-semibold text-text">
              {phase === 'done' ? '◆' : '◇'} {pod}
            </h2>
            <p className="text-xs text-text-muted mt-0.5 font-mono">{cluster} / {namespace}</p>
          </div>
          <button
            onClick={onClose}
            className="w-8 h-8 flex items-center justify-center rounded-sm border border-border
                       hover:bg-hover hover:text-text text-text-muted transition-colors cursor-pointer bg-transparent"
          >
            ✕
          </button>
        </div>

        <div className="p-6 space-y-5">
          {/* Cached notice */}
          {phase === 'done' && cacheRef.current.has(cacheKey) && (
            <div className="flex items-center gap-3 px-4 py-3 rounded-sm bg-elevated border border-border/50 text-sm text-text-secondary">
              <span>◇ From cache</span>
              <button
                onClick={handleRediagnose}
                className="ml-auto text-xs text-accent hover:text-accent/80 transition-colors cursor-pointer bg-transparent border-none"
              >
                ⟳ Re-diagnose
              </button>
            </div>
          )}

          {/* Progress (show during running or idle) */}
          {phase !== 'done' && (
            <div className="space-y-3">
              {diagnosisSteps.map((step, i) => {
                const isActive = currentStep === i + 1;
                const isDone = currentStep > i + 1;
                return (
                  <div
                    key={step.id}
                    className={`flex items-center gap-3 px-4 py-3 rounded-sm border transition-all duration-300 ${
                      isActive
                        ? 'border-accent/30 bg-accent-dim/10'
                        : isDone
                        ? 'border-green/20 bg-green-dim/10'
                        : 'border-border/50 bg-transparent'
                    }`}
                  >
                    <span className={`w-6 h-6 flex items-center justify-center rounded-full text-xs font-mono shrink-0
                      ${isActive ? 'bg-accent text-bg' : isDone ? 'bg-green text-bg' : 'bg-elevated text-text-muted border border-border'}`}
                    >
                      {isDone ? '✓' : isActive ? '◎' : step.id}
                    </span>
                    <div className="flex-1 min-w-0">
                      <p className={`text-sm ${isActive ? 'text-text font-medium' : isDone ? 'text-green' : 'text-text-muted'}`}>
                        {step.label}
                      </p>
                      <p className={`text-xs mt-0.5 font-mono ${isActive ? 'text-accent' : 'text-text-muted'}`}>
                        {isActive ? step.detail : isDone ? 'Complete' : 'Waiting...'}
                      </p>
                    </div>
                    {isActive && <span className="w-1.5 h-1.5 rounded-full bg-accent animate-ping" />}
                  </div>
                );
              })}
            </div>
          )}

          {/* Report */}
          {phase === 'done' && result && (
            <div className="space-y-5 animate-fade-in">
              {/* Timing */}
              <div className="flex items-center gap-1.5 text-xs text-text-muted font-mono">
                <span>⏱</span> Completed in {result.duration}
              </div>

              {/* Root Cause */}
              <div className="px-5 py-4 rounded-sm bg-red-dim/15 border border-red/20">
                <p className="text-xs text-red font-semibold tracking-wider uppercase mb-1">Root Cause</p>
                <p className="text-base font-semibold text-text">{result.rootCause}</p>
                <p className="text-sm text-text-secondary mt-1">Confidence: {result.confidence}</p>
              </div>

              {/* Evidence */}
              <div>
                <h3 className="text-xs text-text-muted font-semibold tracking-wider uppercase mb-3">Evidence Chain</h3>
                <div className="space-y-2">
                  {result.evidence.map((e) => (
                    <div key={e.num} className="flex items-start gap-3 px-4 py-3 rounded-sm bg-elevated border border-border/30">
                      <span className="text-xs text-accent font-semibold font-mono shrink-0 w-5">{e.num}.</span>
                      <span className="text-sm text-text-secondary leading-relaxed">{e.text}</span>
                    </div>
                  ))}
                </div>
              </div>

              {/* Fix */}
              <div>
                <h3 className="text-xs text-text-muted font-semibold tracking-wider uppercase mb-3">Fix Suggestion</h3>
                <div className="px-4 py-3 rounded-sm bg-elevated border border-border">
                  <pre className="text-sm text-accent font-mono whitespace-pre-wrap leading-relaxed">{result.fixCommand}</pre>
                </div>
                <div className="flex items-center gap-3 mt-3">
                  <button
                    onClick={handleCopy}
                    className="text-xs px-3 py-1.5 rounded-sm border border-accent/30 text-accent
                               hover:bg-accent-dim/15 transition-colors cursor-pointer bg-transparent"
                  >
                    {copied ? '✓ Copied' : 'Copy Command'}
                  </button>
                  <button className="text-xs px-3 py-1.5 rounded-sm border border-border text-text-muted
                                     hover:bg-hover hover:text-text transition-colors cursor-pointer bg-transparent">
                    View Raw Data
                  </button>
                </div>
              </div>

              {/* Impact note */}
              <div className="px-4 py-3 rounded-sm bg-elevated border border-border/50 text-sm text-text-muted leading-relaxed">
                ⚠ Impact: {result.impact}
              </div>
            </div>
          )}
        </div>
      </div>
    </>
  );
}
