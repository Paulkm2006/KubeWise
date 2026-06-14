import { useState } from 'react';
import { useTranslation } from 'react-i18next';
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
  const { t } = useTranslation();
  const [manual, setManual] = useState(false);
  const [repoUrl, setRepoUrl] = useState('');
  const [chartName, setChartName] = useState('');

  return (
    <div className="mt-3 p-4 rounded-sm border border-accent/25 bg-accent-dim/10 space-y-3">
      <div className="text-xs font-mono text-accent uppercase tracking-wide">{t('deploy.selectChart')}</div>
      {appName && <div className="text-sm text-text-secondary">{t('deploy.app')} {appName}</div>}

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
              {t('deploy.manual')}
            </button>
            <button
              type="button"
              disabled={disabled}
              onClick={onCancel}
              className="text-sm px-3 py-1.5 rounded-sm border border-border text-text-muted"
            >
              {t('deploy.cancel')}
            </button>
          </div>
        </>
      ) : (
        <div className="space-y-2">
          <input
            value={repoUrl}
            onChange={(e) => setRepoUrl(e.target.value)}
            placeholder={t('deploy.repoUrl')}
            className="w-full text-sm bg-bg border border-border rounded-sm px-3 py-2"
          />
          <input
            value={chartName}
            onChange={(e) => setChartName(e.target.value)}
            placeholder={t('deploy.chartName')}
            className="w-full text-sm bg-bg border border-border rounded-sm px-3 py-2"
          />
          <div className="flex gap-2">
            <button
              type="button"
              disabled={disabled || !repoUrl || !chartName}
              onClick={() => onManual(repoUrl.trim(), chartName.trim())}
              className="text-sm px-3 py-1.5 rounded-sm bg-accent text-bg font-medium disabled:opacity-40"
            >
              {t('deploy.useChart')}
            </button>
            <button type="button" onClick={() => setManual(false)} className="text-sm px-3 py-1.5 rounded-sm border border-border">
              {t('deploy.back')}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

type ValuesTab = 'override' | 'defaults';

interface DeployConfirmCardProps {
  plan: DeployPlan;
  disabled?: boolean;
  onExecute: (values: string) => void;
  onCancel: () => void;
  onCorrect: (values: string, correction: string) => void;
}

export function DeployConfirmCard({ plan, disabled, onExecute, onCancel, onCorrect }: DeployConfirmCardProps) {
  const { t } = useTranslation();
  const [values, setValues] = useState(plan.CustomValues || '');
  const [correction, setCorrection] = useState('');
  const [tab, setTab] = useState<ValuesTab>('override');

  const defaults = plan.DefaultValues || '';
  const chartName = plan.ChartInfo?.ChartName || 'chart';
  const release = plan.ReleaseName || 'release';
  const namespace = plan.Namespace || 'default';

  return (
    <div className="mt-3 w-full min-w-0 p-4 rounded-sm border border-accent/30 bg-accent-dim/10 space-y-4">
      <div className="space-y-1">
        <div className="text-xs font-mono text-accent uppercase tracking-wide">{t('deploy.review')}</div>
        <div className="text-sm text-text flex flex-wrap items-center gap-x-2 gap-y-1">
          <span className="font-medium">{chartName}</span>
          <span className="text-text-muted">→</span>
          <span className="font-mono text-text-secondary">{namespace}/{release}</span>
          {plan.IsUpgrade && (
            <span className="text-xs px-1.5 py-0.5 rounded-sm bg-amber-dim/40 text-amber border border-amber/20">
              {t('deploy.upgrade')}
            </span>
          )}
        </div>
      </div>

      {plan.Warnings && plan.Warnings.length > 0 && (
        <div className="space-y-1">
          {plan.Warnings.map((w, i) => (
            <div key={i} className="text-xs text-amber bg-amber-dim/30 px-2 py-1.5 rounded-sm border border-amber/15">
              {w.Message}
            </div>
          ))}
        </div>
      )}

      <div className="space-y-2">
        <div className="flex gap-1 border-b border-border">
          <button
            type="button"
            onClick={() => setTab('override')}
            className={`text-sm px-3 py-2 -mb-px border-b-2 transition-colors ${
              tab === 'override'
                ? 'border-accent text-accent'
                : 'border-transparent text-text-muted hover:text-text-secondary'
            }`}
          >
            {t('deploy.override')}
            <span className="ml-1.5 text-xs text-text-muted">{t('deploy.editable')}</span>
          </button>
          <button
            type="button"
            onClick={() => setTab('defaults')}
            className={`text-sm px-3 py-2 -mb-px border-b-2 transition-colors ${
              tab === 'defaults'
                ? 'border-accent text-accent'
                : 'border-transparent text-text-muted hover:text-text-secondary'
            }`}
          >
            {t('deploy.defaults')}
            <span className="ml-1.5 text-xs text-text-muted">{t('deploy.readonly')}</span>
          </button>
        </div>

        {tab === 'override' ? (
          <textarea
            value={values}
            onChange={(e) => setValues(e.target.value)}
            rows={14}
            spellCheck={false}
            disabled={disabled}
            className="w-full text-xs font-mono leading-relaxed bg-bg border border-border rounded-sm p-3 text-text-secondary
                       outline-none focus:border-accent/40 disabled:opacity-50 resize-y min-h-[200px]"
            placeholder={t('deploy.valuesPlaceholder')}
          />
        ) : (
          <pre
            className="w-full text-xs font-mono leading-relaxed bg-bg border border-border rounded-sm p-3 text-text-muted
                       overflow-auto max-h-80 min-h-[200px] whitespace-pre-wrap"
          >
            {defaults || '(no default values)'}
          </pre>
        )}
      </div>

      <div className="space-y-2 pt-1 border-t border-border/60">
        <div className="text-sm font-medium text-text">{t('deploy.nlCorrection')}</div>
        <p className="text-xs text-text-muted">
          {t('deploy.nlDesc')}
        </p>
        <textarea
          value={correction}
          onChange={(e) => setCorrection(e.target.value)}
          rows={2}
          disabled={disabled}
          placeholder={t('deploy.nlPlaceholder')}
          className="w-full text-sm bg-bg border border-border rounded-sm px-3 py-2 text-text
                     outline-none focus:border-accent/40 disabled:opacity-50 resize-y"
          onKeyDown={(e) => {
            if (e.key === 'Enter' && (e.metaKey || e.ctrlKey) && correction.trim()) {
              e.preventDefault();
              onCorrect(values, correction.trim());
              setCorrection('');
            }
          }}
        />
        <button
          type="button"
          disabled={disabled || !correction.trim()}
          onClick={() => {
            onCorrect(values, correction.trim());
            setCorrection('');
          }}
          className="text-sm px-4 py-2 rounded-sm bg-accent text-bg font-medium disabled:opacity-40"
        >
          {t('deploy.regenerate')}
        </button>
      </div>

      <div className="flex flex-wrap items-center gap-2 pt-2 border-t border-border/60">
        <button
          type="button"
          disabled={disabled}
          onClick={() => onExecute(values)}
          className="text-sm px-4 py-2 rounded-sm bg-green text-bg font-medium disabled:opacity-40"
        >
          {t('deploy.deployBtn')}
        </button>
        <button
          type="button"
          disabled={disabled}
          onClick={onCancel}
          className="text-sm px-4 py-2 rounded-sm border border-border text-text-muted hover:border-red/30 hover:text-red"
        >
          {t('deploy.cancelDeploy')}
        </button>
        <span className="text-xs text-text-muted ml-auto hidden sm:inline">
          {t('deploy.tip')}
        </span>
      </div>
    </div>
  );
}
