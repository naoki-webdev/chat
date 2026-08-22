import type { KeyboardEvent as ReactKeyboardEvent, MutableRefObject } from 'react'
import type { ApiUser } from '../services/chatApi'
import type { Channel, Message } from '../types/chat'
import { Icon, Avatar } from './ChatIcons'
import { t } from '../i18n'

type Props = {
  selectedChannel: Channel
  visibleMessages: Message[]
  currentUser: ApiUser
  backendAvailable: boolean
  errorMessage?: string
  searchOpen: boolean
  searchQuery: string
  showDetails: boolean
  editingId: string | null
  draft: string
  editDraft: string
  messageListRef: MutableRefObject<HTMLDivElement | null>
  messageElementsRef: MutableRefObject<Record<string, HTMLElement | null>>
  highlightedMessageId: string | null
  hasMore: boolean
  loadingOlder: boolean
  onLoadOlder: () => void
  onSearchOpenChange: (open: boolean) => void
  onSearchQueryChange: (query: string) => void
  onToggleDetails: () => void
  canEditChannel: boolean
  onOpenChannelEdit: () => void
  onToggleReaction: (messageId: string, emoji: string) => void
  savedMessageIds: Set<string>
  onToggleSaved: (messageId: string) => void
  onOpenThread: (message: Message) => void
  typingLabel?: string
  onStartEditing: (message: Message) => void
  onDeleteMessage: (messageId: string) => void
  onDraftChange: (draft: string) => void
  onEditDraftChange: (draft: string) => void
  onComposerKeyDown: (event: ReactKeyboardEvent<HTMLTextAreaElement>) => void
  onSubmit: () => void
  onCancelEditing: () => void
}

export function ChatPanel({ selectedChannel, visibleMessages, currentUser, backendAvailable, errorMessage, searchOpen, searchQuery, showDetails, editingId, draft, editDraft, messageListRef, messageElementsRef, highlightedMessageId, hasMore, loadingOlder, onLoadOlder, onSearchOpenChange, onSearchQueryChange, onToggleDetails, canEditChannel, onOpenChannelEdit, onToggleReaction, savedMessageIds, onToggleSaved, onOpenThread, typingLabel, onStartEditing, onDeleteMessage, onDraftChange, onEditDraftChange, onComposerKeyDown, onSubmit, onCancelEditing }: Props) {
  return <main className={`chat-panel ${showDetails ? 'chat-panel-with-details' : ''}`}>
    <header className="chat-header">
      <div className="channel-title"><span className="channel-title-icon">{selectedChannel.kind === 'channel' ? '#' : '@'}</span><div><h2>{selectedChannel.name}</h2><p>{selectedChannel.description ?? t('chat.directMessageFallback')}</p></div></div>
      <div className="header-actions">
        {searchOpen && <div className="header-search"><Icon name="search" size={16} /><input autoFocus value={searchQuery} onChange={(event) => onSearchQueryChange(event.target.value)} placeholder={t('chat.searchPlaceholder')} /><button onClick={() => { onSearchQueryChange(''); onSearchOpenChange(false) }} aria-label={t('chat.closeSearch')}>×</button></div>}
        <button className="header-icon-button" onClick={() => onSearchOpenChange(!searchOpen)} aria-label={t('chat.searchMessages')}><Icon name="search" size={18} /></button>
        <button className="header-icon-button" onClick={onToggleDetails} aria-label={t('chat.showMembers')}><Icon name="members" size={19} /></button>
        {canEditChannel && <button className="header-icon-button" onClick={onOpenChannelEdit} aria-label={t('chat.editChannel')}><Icon name="settings" size={18} /></button>}
      </div>
    </header>
    <div className="message-list" ref={messageListRef}>
      {hasMore && <button className="load-older-button" onClick={onLoadOlder} disabled={loadingOlder}>{loadingOlder ? t('chat.loading') : t('chat.loadOlder')}</button>}
      <div className="channel-intro"><div className="channel-intro-icon">{selectedChannel.kind === 'channel' ? <Icon name="hash" size={31} /> : <Icon name="message" size={31} />}</div><h3>{selectedChannel.kind === 'channel' ? t('chat.welcomeChannel', { name: selectedChannel.name }) : t('chat.welcomeConversation', { name: selectedChannel.name })}</h3><p>{selectedChannel.kind === 'channel' ? `${selectedChannel.description}。${t('chat.startConversation')}` : t('chat.privateConversation')}</p><div className="intro-line" /></div>
      {searchQuery && <div className="search-summary"><Icon name="search" size={15} /> {t('chat.searchResult', { count: visibleMessages.length })}</div>}
      {visibleMessages.length === 0 && <div className="empty-state"><Icon name="message" size={27} /><p>{t('chat.noMessages')}</p><span>{t('chat.firstMessage')}</span></div>}
      {visibleMessages.map((message, index) => {
        const previous = visibleMessages[index - 1]
        const showAuthor = !previous || previous.author !== message.author
        const isMine = message.authorID === currentUser.id
        const saved = savedMessageIds.has(message.id)
        return <article ref={(element) => { messageElementsRef.current[message.id] = element }} className={`message-row ${showAuthor ? 'message-row-new' : 'message-row-compact'} ${highlightedMessageId === message.id ? 'message-row-highlighted' : ''}`} key={message.id}>
          {showAuthor ? <Avatar initials={message.initials} color={message.color} size="medium" /> : <span className="compact-time">{message.time}</span>}
          <div className="message-content">
            {showAuthor && <div className="message-meta"><strong>{message.author}</strong>{isMine && <span className="you-label">{t('chat.mine')}</span>}<time>{message.time}</time></div>}
            <div className={`message-body ${message.streaming ? 'message-body-streaming' : ''} ${message.aiError ? 'message-body-ai-error' : ''} ${message.deleted ? 'message-body-deleted' : ''}`}>{message.body}{message.streaming && <span className="ai-streaming-cursor" aria-label={t('chat.aiAnswering')} />}{message.edited && <span className="edited-label">{t('chat.edited')}</span>}</div>
            {!message.deleted && !!message.reactions?.length && <div className="reaction-row">{message.reactions.map((reaction) => <button className={`reaction ${reaction.reacted ? 'reaction-active' : ''}`} key={reaction.emoji} onClick={() => onToggleReaction(message.id, reaction.emoji)}>{reaction.emoji} <span>{reaction.count}</span></button>)}<button className="reaction-add" onClick={() => onToggleReaction(message.id, '👍')} aria-label={t('chat.addReaction')}>+</button></div>}
            {!!message.threadCount && <button className="thread-link" onClick={() => onOpenThread(message)}><Icon name="thread" size={15} />{t('chat.replies', { count: message.threadCount })} <span>→</span></button>}
            {!message.threadCount && <button className="thread-link thread-link-empty" onClick={() => onOpenThread(message)}><Icon name="thread" size={15} />{t('chat.reply')}</button>}
          </div>
          <div className="message-actions">{!message.deleted && <><button onClick={() => onToggleReaction(message.id, '👍')} aria-label={t('chat.like')}><span>👍</span></button><button onClick={() => onToggleReaction(message.id, '❤️')} aria-label={t('chat.heart')}><span>♥</span></button></>}<button onClick={() => onToggleSaved(message.id)} aria-label={saved ? t('chat.removeSaved') : t('chat.save')}><Icon name="bookmark" size={15} /></button>{isMine && !message.deleted && <><button onClick={() => onStartEditing(message)} aria-label={t('chat.edit')}><Icon name="edit" size={15} /></button><button onClick={() => onDeleteMessage(message.id)} aria-label={t('chat.delete')}><Icon name="trash" size={15} /></button></>}</div>
        </article>
      })}
      {typingLabel && <div className="typing-indicator"><span className="typing-dots"><i /><i /><i /></span>{t('chat.typing', { name: typingLabel })}</div>}
    </div>
    <div className="composer-wrap">
      {errorMessage && <div className="composer-error" role="alert">{errorMessage}</div>}
      {editingId && <div className="editing-bar"><Icon name="edit" size={14} /><span>{t('chat.editing')}</span><button onClick={onCancelEditing}>{t('chat.cancel')}</button></div>}
      <div className={`composer ${editingId ? 'composer-editing' : ''}`}><textarea disabled={!backendAvailable} value={editingId ? editDraft : draft} onChange={(event) => editingId ? onEditDraftChange(event.target.value) : onDraftChange(event.target.value)} onKeyDown={onComposerKeyDown} placeholder={t('chat.sendPlaceholder', { prefix: selectedChannel.kind === 'channel' ? '#' : '@', name: selectedChannel.name })} rows={1} /><div className="composer-tools composer-tools-right"><button disabled={!backendAvailable || !(editingId ? editDraft : draft).trim()} className={`send-button ${(editingId ? editDraft : draft).trim() ? 'send-button-active' : ''}`} onClick={onSubmit} aria-label={editingId ? t('chat.saveEdit') : t('chat.send')}>{editingId ? <Icon name="check" size={18} /> : <Icon name="send" size={18} />}</button></div></div>
      <div className="composer-hint"><span>{t('chat.sendWithEnter')}</span><span>{t('chat.newlineWithShiftEnter')}</span></div>
    </div>
  </main>
}
