import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import type { Components } from 'react-markdown';

const mdComponents: Components = {
  h1: ({ children }) => (
    <h1 className="text-lg font-semibold text-text mt-4 mb-2 first:mt-0">{children}</h1>
  ),
  h2: ({ children }) => (
    <h2 className="text-base font-semibold text-text mt-3 mb-2 first:mt-0">{children}</h2>
  ),
  h3: ({ children }) => (
    <h3 className="text-sm font-semibold text-text mt-3 mb-1.5 first:mt-0">{children}</h3>
  ),
  p: ({ children }) => <p className="mb-2 last:mb-0">{children}</p>,
  ul: ({ children }) => <ul className="list-disc pl-5 mb-2 space-y-1">{children}</ul>,
  ol: ({ children }) => <ol className="list-decimal pl-5 mb-2 space-y-1">{children}</ol>,
  li: ({ children }) => <li className="text-text-secondary">{children}</li>,
  strong: ({ children }) => <strong className="font-semibold text-text">{children}</strong>,
  em: ({ children }) => <em className="italic text-text-secondary">{children}</em>,
  a: ({ href, children }) => (
    <a href={href} className="text-accent hover:underline" target="_blank" rel="noreferrer">
      {children}
    </a>
  ),
  blockquote: ({ children }) => (
    <blockquote className="border-l-2 border-accent/40 pl-3 my-2 text-text-muted italic">{children}</blockquote>
  ),
  hr: () => <hr className="border-border my-3" />,
  table: ({ children }) => (
    <div className="overflow-x-auto my-2">
      <table className="w-full text-xs border-collapse border border-border">{children}</table>
    </div>
  ),
  thead: ({ children }) => <thead className="bg-bg">{children}</thead>,
  th: ({ children }) => (
    <th className="border border-border px-2 py-1.5 text-left font-medium text-text">{children}</th>
  ),
  td: ({ children }) => (
    <td className="border border-border px-2 py-1.5 text-text-secondary">{children}</td>
  ),
  code: ({ className, children }) => {
    const isBlock = className?.includes('language-');
    if (isBlock) {
      return <code className={`${className} font-mono text-xs text-text-secondary`}>{children}</code>;
    }
    return (
      <code className="font-mono text-xs text-accent bg-bg border border-border/60 px-1 py-0.5 rounded-sm">
        {children}
      </code>
    );
  },
  pre: ({ children }) => (
    <pre className="bg-bg border border-border rounded-sm p-3 overflow-x-auto my-2 text-xs font-mono leading-relaxed">
      {children}
    </pre>
  ),
};

interface MarkdownContentProps {
  content: string;
  className?: string;
}

export default function MarkdownContent({ content, className = '' }: MarkdownContentProps) {
  if (!content.trim()) return null;

  return (
    <div className={`markdown-content ${className}`}>
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={mdComponents}>
        {content}
      </ReactMarkdown>
    </div>
  );
}
