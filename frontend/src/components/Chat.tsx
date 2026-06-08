import { useState, useRef, useEffect } from 'react';
import { ChatMessage, chatResponses } from '../data/mock';

export default function Chat() {
  const [messages, setMessages] = useState<ChatMessage[]>([
    { id: 'welcome', role: 'ai', text: `Hello, I'm KubeWise AI. I can help you manage your Kubernetes clusters across all environments.`, timestamp: '09:41:23' },
  ]);
  const [input, setInput] = useState('');
  const [typing, setTyping] = useState(false);
  const endRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, typing]);

  const sendMessage = (text: string) => {
    if (!text.trim() || typing) return;
    const now = new Date();
    const ts = `${now.getHours().toString().padStart(2, '0')}:${now.getMinutes().toString().padStart(2, '0')}:${now.getSeconds().toString().padStart(2, '0')}`;

    const userMsg: ChatMessage = {
      id: `msg-${Date.now()}`,
      role: 'user',
      text: text.trim(),
      timestamp: ts,
      cluster: 'prod-us',
    };

    setMessages((prev) => [...prev, userMsg]);
    setInput('');
    setTyping(true);

    // Find response
    const response = chatResponses[text.trim()] || `I've analyzed your request. Here's what I found:

• Scanned **4 clusters** (prod-cn, prod-us, staging, dev)
• **2 issues** detected on prod-us
• Top issue: \`nginx-7d9f\` in CrashLoopBackOff (exit code 137 — OOMKilled)

Would you like me to run a full diagnosis?`;

    // Simulate typing delay
    setTimeout(() => {
      const aiMsg: ChatMessage = {
        id: `msg-${Date.now()}`,
        role: 'ai',
        text: response,
        timestamp: ts,
      };
      setMessages((prev) => [...prev, aiMsg]);
      setTyping(false);
    }, 800 + Math.random() * 1200);
  };

  return (
    <div className="h-full flex flex-col px-8 py-6">
      {/* Messages */}
      <div className="flex-1 overflow-y-auto space-y-4 pb-4">
        {messages.map((msg) => (
          <div key={msg.id} className={`flex gap-4 ${msg.role === 'user' ? 'flex-row-reverse' : ''}`}>
            <div className={`w-10 h-10 rounded-full flex items-center justify-center text-sm font-semibold shrink-0 mt-0.5 border shrink-0
              ${msg.role === 'ai'
                ? 'bg-accent-dim/20 text-accent border-accent/20'
                : 'bg-accent-dim/10 text-accent/70 border-border'
              }`}>
              {msg.role === 'ai' ? 'KW' : 'U'}
            </div>
            <div className={`max-w-[65%] space-y-1.5`}>
              <div className={`p-4 rounded-sm text-sm leading-relaxed whitespace-pre-wrap
                ${msg.role === 'ai'
                  ? 'bg-elevated border border-border text-text-secondary'
                  : 'bg-accent-dim/10 border border-accent/15 text-text'
                }`}>
                {msg.text}
              </div>
              <p className={`text-xs text-text-muted font-mono ${msg.role === 'user' ? 'text-right' : ''}`}>
                {msg.timestamp}
              </p>
            </div>
          </div>
        ))}

        {/* Typing indicator */}
        {typing && (
          <div className="flex gap-4">
            <div className="w-10 h-10 rounded-full bg-accent-dim/20 text-accent flex items-center justify-center text-sm font-semibold shrink-0 mt-0.5 border border-accent/20">
              KW
            </div>
            <div className="max-w-[65%]">
              <div className="p-4 rounded-sm bg-elevated border border-border">
                <div className="flex gap-1.5">
                  <span className="w-2 h-2 rounded-full bg-text-muted animate-bounce" style={{ animationDelay: '0ms' }} />
                  <span className="w-2 h-2 rounded-full bg-text-muted animate-bounce" style={{ animationDelay: '200ms' }} />
                  <span className="w-2 h-2 rounded-full bg-text-muted animate-bounce" style={{ animationDelay: '400ms' }} />
                </div>
              </div>
            </div>
          </div>
        )}

        <div ref={endRef} />
      </div>

      {/* Suggestions */}
      <div className="flex gap-2 mb-4 flex-wrap">
        {Object.keys(chatResponses).map((s) => (
          <button
            key={s}
            onClick={() => sendMessage(s)}
            className="text-sm text-text-muted px-3 py-1.5 rounded-sm border border-border
                       hover:border-accent/30 hover:text-accent transition-colors cursor-pointer bg-transparent"
          >
            {s}
          </button>
        ))}
      </div>

      {/* Input */}
      <div className="flex items-center gap-3 pt-4 border-t border-border">
        <span className="text-sm text-text-muted px-3 py-2 border border-border rounded-sm shrink-0 font-mono">
          ▼ prod-us
        </span>
        <input
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && sendMessage(input)}
          placeholder="Type a message..."
          className="flex-1 bg-surface border border-border rounded-sm px-4 py-2.5 text-sm text-text
                     placeholder:text-text-muted outline-none focus:border-accent/30 transition-colors font-sans"
        />
        <button
          onClick={() => sendMessage(input)}
          disabled={typing || !input.trim()}
          className="text-sm font-medium px-4 py-2.5 rounded-sm bg-accent text-bg
                     hover:opacity-85 disabled:opacity-40 transition-opacity cursor-pointer border-none shrink-0"
        >
          Send →
        </button>
      </div>
    </div>
  );
}
