import type { ChartCandidate, ChatProgressCard, OperationStep } from '../../api/types';
import { toggleDetailsExpanded, togglePhaseExpanded } from '../../lib/chatFold';
import MarkdownContent from '../MarkdownContent';
import ConfirmCard from './ConfirmCard';
import { ChartSelectCard, DeployConfirmCard } from './DeployCards';

interface ProgressCardProps {
  card: ChatProgressCard;
  cluster: string;
  interactionPending?: boolean;
  onTogglePhase: (phaseId: string) => void;
  onToggleDetails: () => void;
  onInteractionConfirm: () => void;
  onInteractionSkip: () => void;
  onInteractionCorrect: (correction: string) => void;
  onChartSelect: (index: number) => void;
  onChartManual: (repoUrl: string, chartName: string) => void;
  onChartCancel: () => void;
  onDeployExecute: (values: string) => void;
  onDeployCancel: () => void;
  onDeployCorrect: (values: string, correction: string) => void;
}

function statusDot(done: boolean, failed: boolean, running: boolean) {
  if (failed) return 'bg-red';
  if (done) return 'bg-green';
  if (running) return 'bg-accent animate-pulse';
  return 'bg-text-muted/40';
}

export default function ProgressCard({
  card,
  cluster,
  interactionPending,
  onTogglePhase,
  onToggleDetails,
  onInteractionConfirm,
  onInteractionSkip,
  onInteractionCorrect,
  onChartSelect,
  onChartManual,
  onChartCancel,
  onDeployExecute,
  onDeployCancel,
  onDeployCorrect,
}: ProgressCardProps) {
  const running = !card.done && !card.failed;
  const interaction = card.awaitingInteraction && !card.awaitingInteraction.resolved
    ? card.awaitingInteraction
    : undefined;

  const report = card.finalReport || (card.failed ? card.errorMsg : '') || '';

  return (
    <div className="p-4 rounded-sm bg-elevated border border-border space-y-3">
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2 min-w-0">
          <span className={`w-2 h-2 rounded-full shrink-0 ${statusDot(card.done, card.failed, running)}`} />
          <span className="text-sm font-medium text-text truncate">{card.agentName || 'Agent'}</span>
        </div>
        <span className="text-xs font-mono text-text-muted shrink-0">◆ {cluster}</span>
      </div>

      {report && (
        <div className="space-y-1">
          <div className="text-xs font-mono text-accent uppercase tracking-wide">Report</div>
          <MarkdownContent content={report} />
        </div>
      )}

      <div>
        <button
          type="button"
          onClick={onToggleDetails}
          className="text-xs text-text-muted hover:text-accent font-mono"
        >
          {card.detailsExpanded ? '▼' : '▶'} Execution details
          {card.durationMs != null && ` · ${(card.durationMs / 1000).toFixed(1)}s`}
        </button>

        {card.detailsExpanded && (
          <div className="mt-2 space-y-2 border-l-2 border-border pl-3">
            {card.phases.map((phase) => (
              <div key={phase.id} className="space-y-1">
                <button
                  type="button"
                  onClick={() => onTogglePhase(phase.id)}
                  className="flex items-center gap-2 text-sm text-text-secondary hover:text-text w-full text-left"
                >
                  <span className={`w-1.5 h-1.5 rounded-full ${statusDot(phase.done, false, !phase.done && running)}`} />
                  <span className="font-mono text-xs">{phase.expanded ? '▼' : '▶'}</span>
                  <span className="truncate">{phase.label}</span>
                </button>

                {phase.expanded && (
                  <div className="ml-5 space-y-2 pb-2">
                    {phase.tools.map((t, i) => (
                      <div key={`${t.name}-${t.step}-${i}`} className="flex items-center gap-2 text-xs font-mono text-text-muted">
                        <span className={t.failed ? 'text-red' : t.done ? 'text-green' : 'text-accent'}>
                          {t.failed ? '✗' : t.done ? '✓' : '…'}
                        </span>
                        <span>{t.name}</span>
                        {t.elapsedMs != null && <span>{t.elapsedMs}ms</span>}
                      </div>
                    ))}
                    {phase.reasoningText && (
                      <div className="max-h-48 overflow-y-auto bg-bg/50 p-2 rounded-sm border border-border/50">
                        <MarkdownContent content={phase.reasoningText} className="text-xs" />
                      </div>
                    )}
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      {interaction?.kind === 'operation_step' && (
        <ConfirmCard
          step={interaction.payload as unknown as OperationStep}
          totalSteps={interaction.totalSteps}
          disabled={interactionPending}
          onConfirm={onInteractionConfirm}
          onSkip={onInteractionSkip}
          onCorrect={onInteractionCorrect}
        />
      )}

      {interaction?.kind === 'chart_select' && (
        <ChartSelectCard
          appName={String(interaction.payload.app_name || '')}
          candidates={(interaction.payload.candidates as ChartCandidate[]) || []}
          disabled={interactionPending}
          onSelect={onChartSelect}
          onManual={onChartManual}
          onCancel={onChartCancel}
        />
      )}

      {interaction?.kind === 'deploy_confirm' && (
        <div className="-mx-1 w-[min(100%,42rem)]">
          <DeployConfirmCard
            plan={interaction.payload as unknown as DeployPlan}
            disabled={interactionPending}
            onExecute={onDeployExecute}
            onCancel={onDeployCancel}
            onCorrect={onDeployCorrect}
          />
        </div>
      )}

      {card.failed && card.errorMsg && !report && (
        <p className="text-sm text-red">{card.errorMsg}</p>
      )}
    </div>
  );
}
