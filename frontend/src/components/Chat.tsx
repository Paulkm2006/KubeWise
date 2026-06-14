import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { api } from '../api/client';
import { subscribeChat } from '../api/chatSse';
import type { ChatProgressCard, ChatSSEEvent, ChatTurn } from '../api/types';
import {
  createProgressCard,
  foldChatEvent,
  toggleDetailsExpanded,
  togglePhaseExpanded,
} from '../lib/chatFold';
import MarkdownContent from './MarkdownContent';
import ProgressCard from './chat/ProgressCard';

interface ChatProps {
  activeCluster: string;
}

function formatTime(d = new Date()): string {
  return `${d.getHours().toString().padStart(2, '0')}:${d.getMinutes().toString().padStart(2, '0')}:${d.getSeconds().toString().padStart(2, '0')}`;
}

export default function Chat({ activeCluster }: ChatProps) {
  const { t } = useTranslation();

  const SUGGESTIONS = [
    t('chat.suggestions.namespaces'),
    t('chat.suggestions.pvc'),
    t('chat.suggestions.audit'),
    t('chat.suggestions.deploy'),
  ];

  const [turns, setTurns] = useState<ChatTurn[]>([
    {
      id: 'welcome',
      role: 'assistant',
      text: t('chat.welcome'),
      cluster: '',
      timestamp: formatTime(),
    },
  ]);
  const [input, setInput] = useState('');
  const [streaming, setStreaming] = useState(false);
  const [liveCard, setLiveCard] = useState<ChatProgressCard | null>(null);
  const [interactionPending, setInteractionPending] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);
  const endRef = useRef<HTMLDivElement>(null);
  const cleanupRef = useRef<(() => void) | null>(null);
  const liveClusterRef = useRef('');
  const pinnedToBottomRef = useRef(true);

  const scrollToEnd = useCallback((behavior: ScrollBehavior = 'smooth') => {
    endRef.current?.scrollIntoView({ behavior });
  }, []);

  const handleScroll = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    pinnedToBottomRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 64;
  }, []);

  // Scroll only when the message list grows and the user was already at the bottom.
  useEffect(() => {
    if (pinnedToBottomRef.current) {
      requestAnimationFrame(() => scrollToEnd());
    }
  }, [turns, scrollToEnd]);

  useEffect(() => () => cleanupRef.current?.(), []);

  const finalizeStream = useCallback((card: ChatProgressCard, cluster: string) => {
    const report = card.finalReport || card.errorMsg || 'Completed without a report.';
    setTurns((prev) => [
      ...prev,
      {
        id: `asst-${Date.now()}`,
        role: card.failed ? 'error' : 'assistant',
        text: report,
        cluster,
        timestamp: formatTime(),
        card: { ...card, finalReport: report, done: true, awaitingInteraction: undefined },
      },
    ]);
    setLiveCard(null);
    setStreaming(false);
  }, []);

  const handleEvent = useCallback((ev: ChatSSEEvent) => {
    setLiveCard((prev) => {
      if (!prev) return prev;
      const next = foldChatEvent(prev, ev);
      return next;
    });
  }, []);

  const sendMessage = useCallback((text: string) => {
    const trimmed = text.trim();
    if (!trimmed || streaming) return;
    if (!activeCluster) return;

    cleanupRef.current?.();
    liveClusterRef.current = activeCluster;

    const userTurn: ChatTurn = {
      id: `user-${Date.now()}`,
      role: 'user',
      text: trimmed,
      cluster: activeCluster,
      timestamp: formatTime(),
    };
    setTurns((prev) => [...prev, userTurn]);
    setInput('');
    setStreaming(true);
    pinnedToBottomRef.current = true;
    requestAnimationFrame(() => scrollToEnd());

    const queryId = `q-${Date.now()}`;
    const card = createProgressCard(queryId, '…');
    setLiveCard(card);

    const url = api.chats.streamUrl({ query: trimmed, query_id: queryId, cluster: activeCluster });
    cleanupRef.current = subscribeChat(url, {
      onEvent: handleEvent,
      onComplete: () => {
        setLiveCard((prev) => {
          if (prev) {
            const card = !prev.done && !prev.failed
              ? foldChatEvent(prev, { type: 'stream_err', error: 'Stream ended without completion' })
              : prev;
            finalizeStream(card, liveClusterRef.current);
          }
          return null;
        });
        cleanupRef.current = null;
      },
      onError: (err) => {
        setLiveCard((prev) => {
          const failed = prev
            ? foldChatEvent(prev, { type: 'stream_err', error: err })
            : createProgressCard(queryId, 'Error');
          finalizeStream(failed, liveClusterRef.current);
          return null;
        });
        cleanupRef.current = null;
      },
    });
  }, [activeCluster, streaming, handleEvent, finalizeStream, scrollToEnd]);

  const answerInteraction = async (payload: unknown) => {
    const interaction = liveCard?.awaitingInteraction;
    if (!interaction || interactionPending) return;
    setInteractionPending(true);
    try {
      await api.chats.answerInteraction({
        interaction_id: interaction.interactionId,
        payload,
      });
      setLiveCard((prev) =>
        prev && prev.awaitingInteraction
          ? { ...prev, awaitingInteraction: { ...prev.awaitingInteraction, resolved: true } }
          : prev,
      );
    } finally {
      setInteractionPending(false);
    }
  };

  const clusterLabel = activeCluster || 'no cluster';

  return (
    <div className="h-full flex flex-col px-8 py-6">
      <div ref={scrollRef} onScroll={handleScroll} className="flex-1 overflow-y-auto space-y-4 pb-4">
        {turns.map((turn) => (
          <div key={turn.id} className={`flex gap-4 ${turn.role === 'user' ? 'flex-row-reverse' : ''}`}>
            <div
              className={`w-10 h-10 rounded-full flex items-center justify-center text-sm font-semibold shrink-0 mt-0.5 border
                ${turn.role === 'user'
                  ? 'bg-accent-dim/10 text-accent/70 border-border'
                  : turn.role === 'error'
                    ? 'bg-red-dim/30 text-red border-red/20'
                    : 'bg-accent-dim/20 text-accent border-accent/20'
                }`}
            >
              {turn.role === 'user' ? 'U' : 'KW'}
            </div>
            <div className={`max-w-[75%] space-y-1.5 min-w-0`}>
              {turn.role === 'user' ? (
                <div className="p-4 rounded-sm text-sm leading-relaxed bg-accent-dim/10 border border-accent/15 text-text">
                  {turn.text}
                </div>
              ) : turn.card ? (
                <ProgressCard
                  card={turn.card}
                  cluster={turn.cluster || activeCluster}
                  onTogglePhase={(phaseId) =>
                    setTurns((prev) =>
                      prev.map((t) =>
                        t.id === turn.id && t.card
                          ? { ...t, card: togglePhaseExpanded(t.card, phaseId) }
                          : t,
                      ),
                    )
                  }
                  onToggleDetails={() =>
                    setTurns((prev) =>
                      prev.map((t) =>
                        t.id === turn.id && t.card
                          ? { ...t, card: toggleDetailsExpanded(t.card) }
                          : t,
                      ),
                    )
                  }
                  onInteractionConfirm={() => {}}
                  onInteractionSkip={() => {}}
                  onInteractionCorrect={() => {}}
                  onChartSelect={() => {}}
                  onChartManual={() => {}}
                  onChartCancel={() => {}}
                  onDeployExecute={() => {}}
                  onDeployCancel={() => {}}
                  onDeployCorrect={() => {}}
                />
              ) : (
                <div className="p-4 rounded-sm bg-elevated border border-border">
                  <MarkdownContent content={turn.text} />
                </div>
              )}
              <p className={`text-xs text-text-muted font-mono flex items-center gap-2 ${turn.role === 'user' ? 'justify-end' : ''}`}>
                {turn.cluster && <span className="text-accent/70">◆ {turn.cluster}</span>}
                <span>{turn.timestamp}</span>
              </p>
            </div>
          </div>
        ))}

        {liveCard && (
          <div className="flex gap-4">
            <div className="w-10 h-10 rounded-full bg-accent-dim/20 text-accent flex items-center justify-center text-sm font-semibold shrink-0 border border-accent/20">
              KW
            </div>
            <div className="max-w-[min(100%,48rem)] min-w-0 flex-1">
              <ProgressCard
                card={liveCard}
                cluster={liveClusterRef.current}
                interactionPending={interactionPending}
                onTogglePhase={(phaseId) => setLiveCard((c) => (c ? togglePhaseExpanded(c, phaseId) : c))}
                onToggleDetails={() => setLiveCard((c) => (c ? toggleDetailsExpanded(c) : c))}
                onInteractionConfirm={() => answerInteraction({ confirmed: true })}
                onInteractionSkip={() => answerInteraction({ confirmed: false })}
                onInteractionCorrect={(correction) => answerInteraction({ confirmed: false, correction })}
                onChartSelect={(index) => answerInteraction({ cancelled: false, candidate_index: index })}
                onChartManual={(repoUrl, chartName) =>
                  answerInteraction({
                    use_manual_chart: true,
                    manual_repo_url: repoUrl,
                    manual_chart_name: chartName,
                  })
                }
                onChartCancel={() => answerInteraction({ cancelled: true })}
                onDeployExecute={(values) => answerInteraction({ action: 'execute', values })}
                onDeployCancel={() => answerInteraction({ action: 'cancel' })}
                onDeployCorrect={(values, correction) =>
                  answerInteraction({ action: 'execute', values, correction })
                }
              />
            </div>
          </div>
        )}

        <div ref={endRef} />
      </div>

      <div className="flex gap-2 mb-4 flex-wrap">
        {SUGGESTIONS.map((s) => (
          <button
            key={s}
            type="button"
            disabled={streaming || !activeCluster}
            onClick={() => sendMessage(s)}
            className="text-sm text-text-muted px-3 py-1.5 rounded-sm border border-border
                       hover:border-accent/30 hover:text-accent transition-colors cursor-pointer bg-transparent
                       disabled:opacity-40"
          >
            {s}
          </button>
        ))}
      </div>

      <div className="flex items-center gap-3 pt-4 border-t border-border">
        <span
          className={`text-sm px-3 py-2 border rounded-sm shrink-0 font-mono
            ${activeCluster ? 'text-accent border-accent/25 bg-accent-dim/10' : 'text-red border-red/30 bg-red-dim/10'}`}
          title={t('chat.clusterContext')}
        >
          ◆ {clusterLabel}
        </span>
        <input
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && sendMessage(input)}
          placeholder={activeCluster ? t('chat.placeholder') : t('chat.noCluster')}
          disabled={streaming || !activeCluster}
          className="flex-1 bg-surface border border-border rounded-sm px-4 py-2.5 text-sm text-text
                     placeholder:text-text-muted outline-none focus:border-accent/30 transition-colors font-sans
                     disabled:opacity-50"
        />
        <button
          type="button"
          onClick={() => sendMessage(input)}
          disabled={streaming || !input.trim() || !activeCluster}
          className="text-sm font-medium px-4 py-2.5 rounded-sm bg-accent text-bg
                     hover:opacity-85 disabled:opacity-40 transition-opacity cursor-pointer border-none shrink-0"
        >
          {t('chat.send')}
        </button>
      </div>
    </div>
  );
}
