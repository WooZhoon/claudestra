import { useState, KeyboardEvent } from 'react';

interface InputBarProps {
  onSubmit: (input: string) => void;
  onCancel?: () => void;
  disabled: boolean;
}

export default function InputBar({ onSubmit, onCancel, disabled }: InputBarProps) {
  const [input, setInput] = useState('');

  const handleSubmit = () => {
    const trimmed = input.trim();
    if (!trimmed || disabled) return;
    onSubmit(trimmed);
    setInput('');
  };

  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSubmit();
    }
  };

  return (
    <div style={{
      padding: '12px 16px',
      borderTop: '1px solid var(--border)',
      background: 'var(--bg-secondary)',
      display: 'flex',
      gap: 8,
    }}>
      <input
        value={input}
        onChange={e => setInput(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder={disabled ? '실행 중...' : '요청을 입력하세요...'}
        disabled={disabled}
        style={{
          flex: 1,
          padding: '10px 14px',
          borderRadius: 8,
          border: '1px solid var(--border)',
          background: 'var(--bg-primary)',
          color: 'var(--text-primary)',
          fontSize: 14,
          outline: 'none',
        }}
      />
      {disabled ? (
        <button
          onClick={onCancel}
          style={{
            padding: '10px 20px',
            borderRadius: 8,
            border: 'none',
            background: '#f7768e',
            color: '#1a1b26',
            fontSize: 14,
            fontWeight: 600,
            cursor: 'pointer',
          }}
        >
          중지
        </button>
      ) : (
        <button
          onClick={handleSubmit}
          disabled={!input.trim()}
          style={{
            padding: '10px 20px',
            borderRadius: 8,
            border: 'none',
            background: !input.trim() ? 'var(--bg-tertiary)' : 'var(--accent)',
            color: !input.trim() ? 'var(--text-muted)' : '#1a1b26',
            fontSize: 14,
            fontWeight: 600,
            cursor: !input.trim() ? 'not-allowed' : 'pointer',
          }}
        >
          전송
        </button>
      )}
    </div>
  );
}
