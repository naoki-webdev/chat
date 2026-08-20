import type { KeyboardEvent } from 'react'
import type { Message } from '../types/chat'
import { Avatar, Icon } from './ChatIcons'
import { t } from '../i18n'

type Props = {
  root: Message
  replies: Message[]
  draft: string
  loading: boolean
  onDraftChange: (value: string) => void
  onKeyDown: (event: KeyboardEvent<HTMLTextAreaElement>) => void
  onSubmit: () => void
  onClose: () => void
}

export function ThreadPanel({ root, replies, draft, loading, onDraftChange, onKeyDown, onSubmit, onClose }: Props) {
  return <aside className="thread-panel">
    <header className="thread-header"><div><span className="eyebrow">{t('thread.eyebrow')}</span><h2>{t('thread.title')}</h2></div><button className="icon-button" onClick={onClose} aria-label={t('thread.close')}>×</button></header>
    <div className="thread-root"><Avatar initials={root.initials} color={root.color} size="small" /><div><strong>{root.author}</strong><p>{root.body}</p></div></div>
    <div className="thread-replies">{loading && <div className="thread-loading">{t('thread.loading')}</div>}{!loading && replies.length === 0 && <div className="thread-empty"><Icon name="thread" size={20} />{t('thread.empty')}</div>}{replies.map((reply) => <article className="thread-reply" key={reply.id}><Avatar initials={reply.initials} color={reply.color} size="small" /><div><div className="thread-reply-meta"><strong>{reply.author}</strong><time>{reply.time}</time></div><p>{reply.body}</p></div></article>)}</div>
    <div className="thread-composer"><textarea value={draft} onChange={(event) => onDraftChange(event.target.value)} onKeyDown={onKeyDown} placeholder={t('thread.placeholder')} rows={2} /><button className={`send-button ${draft.trim() ? 'send-button-active' : ''}`} onClick={onSubmit} aria-label={t('thread.send')}><Icon name="send" size={17} /></button></div>
  </aside>
}
