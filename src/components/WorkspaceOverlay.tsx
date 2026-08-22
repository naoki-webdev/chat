import { useMemo, useState } from 'react'
import type { Channel, Message } from '../types/chat'
import { Avatar, Icon } from './ChatIcons'
import { t } from '../i18n'

export type WorkspaceOverlayKind = 'search' | 'inbox' | 'saved' | 'threads' | 'workspace' | 'help'

export type SavedMessageRef = {
  channelId: string
  messageId: string
}

type Props = {
  kind: WorkspaceOverlayKind
  channels: Channel[]
  messages: Record<string, Message[]>
  savedMessages: SavedMessageRef[]
  onSelectChannel: (channel: Channel) => void
  onOpenThread: (channel: Channel, message: Message) => void
  onClose: () => void
  memberCount?: number
  connection: 'connected' | 'reconnecting'
}

export function WorkspaceOverlay({ kind, channels, messages, savedMessages, onSelectChannel, onOpenThread, onClose, memberCount, connection }: Props) {
  const groupLabel = (group: string) => t(`sidebar.groups.${group}`)
  const [query, setQuery] = useState('')
  const channelById = useMemo(() => new Map(channels.map((channel) => [channel.id, channel])), [channels])
  const allMessages = useMemo(() => Object.entries(messages).flatMap(([channelId, channelMessages]) => channelMessages.map((message) => ({ channelId, message }))), [messages])
  const normalizedQuery = query.trim().toLowerCase()

  const searchResults = useMemo(() => channels.filter((channel) => {
    if (!normalizedQuery) return true
    return `${channel.name} ${channel.description ?? ''} ${channel.group}`.toLowerCase().includes(normalizedQuery)
  }), [channels, normalizedQuery])

  const unreadChannels = channels.filter((channel) => channel.unread > 0)
  const savedItems = savedMessages.map((reference) => ({
    reference,
    channel: channelById.get(reference.channelId),
    message: messages[reference.channelId]?.find((message) => message.id === reference.messageId),
  })).filter((item) => item.channel && item.message)
  const threadItems = allMessages.filter(({ message }) => (message.threadCount ?? 0) > 0)

  const select = (channel: Channel) => {
    onSelectChannel(channel)
    onClose()
  }

  return <div className="workspace-overlay-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
    <section className="workspace-overlay" role="dialog" aria-modal="true" aria-label={kind === 'search' ? t('overlay.searchDialog') : t(`overlay.titles.${kind}`)}>
      <header className="workspace-overlay-header">
        <div>
          <span className="eyebrow">{t('brand.workspaceName')}</span>
          <h2>{kind === 'search' ? t('overlay.searchTitle') : t(`overlay.titles.${kind}`)}</h2>
        </div>
        <button className="icon-button" onClick={onClose} aria-label={t('overlay.close')}>×</button>
      </header>

      {kind === 'search' && <div className="workspace-overlay-search"><Icon name="search" size={16} /><input autoFocus value={query} onChange={(event) => setQuery(event.target.value)} placeholder={t('overlay.searchPlaceholder')} /></div>}

      <div className="workspace-overlay-body">
      {kind === 'search' && <>
          <p className="workspace-overlay-hint">{t('overlay.searchHint')}</p>
          <div className="workspace-result-list">{searchResults.length === 0 && <div className="workspace-overlay-empty">{t('overlay.noResults')}</div>}{searchResults.map((channel) => <button className="workspace-result" key={channel.id} onClick={() => select(channel)}><span className="workspace-result-icon">{channel.kind === 'channel' ? '#' : <Avatar initials={channel.initials ?? ''} color={channel.color ?? '#394b6a'} size="small" />}</span><span><strong>{channel.name}</strong><small>{channel.kind === 'channel' ? groupLabel(channel.group) : t('overlay.directMessageType')}</small></span>{channel.unread > 0 && <span className="unread-badge">{channel.unread}</span>}</button>)}</div>
      </>}

      {kind === 'workspace' && <div className="workspace-info-card"><div className="workspace-info-icon">LL</div><h3>{t('brand.workspaceName')} <span className="verified-mark">✦</span></h3><p>{t('overlay.workspaceDescription')}</p><div className="workspace-info-grid"><div><strong>{channels.length}</strong><small>{t('overlay.conversationCount')}</small></div><div><strong>{memberCount ?? '—'}</strong><small>{t('overlay.memberCount')}</small></div><div><strong>{t(`chat.${connection}`)}</strong><small>{t('overlay.connectionState')}</small></div></div><p className="workspace-overlay-hint">{t('overlay.workspaceHint')}</p></div>}

      {kind === 'help' && <div className="help-list"><div className="help-row"><span>{t('overlay.helpSearch')}</span><kbd>⌘ / Ctrl + K</kbd></div><div className="help-row"><span>{t('overlay.helpClose')}</span><kbd>Esc</kbd></div><div className="help-row"><span>{t('overlay.helpSend')}</span><kbd>Enter</kbd></div><div className="help-row"><span>{t('overlay.helpNewline')}</span><kbd>Shift + Enter</kbd></div><p className="workspace-overlay-hint">{t('overlay.helpDescription')}</p></div>}

        {kind === 'inbox' && <>
          <p className="workspace-overlay-hint">{t('overlay.unreadHint')}</p>
          <div className="workspace-result-list">{unreadChannels.length === 0 && <div className="workspace-overlay-empty"><Icon name="check" size={20} /><span>{t('overlay.allRead')}</span></div>}{unreadChannels.map((channel) => { const channelMessages = messages[channel.id] ?? []; const latest = channelMessages[channelMessages.length - 1]; return <button className="workspace-result workspace-result-message" key={channel.id} onClick={() => select(channel)}><span className="workspace-result-icon">{channel.kind === 'channel' ? '#' : <Avatar initials={channel.initials ?? ''} color={channel.color ?? '#394b6a'} size="small" />}</span><span><strong>{channel.kind === 'channel' ? `#${channel.name}` : channel.name}</strong><small>{latest?.body ?? t('overlay.openUnread')}</small></span><span className="unread-badge">{channel.unread}</span></button> })}</div>
        </>}

        {kind === 'saved' && <>
          <p className="workspace-overlay-hint">{t('overlay.savedHint')}</p>
          <div className="workspace-result-list">{savedItems.length === 0 && <div className="workspace-overlay-empty"><Icon name="bookmark" size={20} /><span>{t('overlay.noSaved')}</span></div>}{savedItems.map(({ reference, channel, message }) => <button className="workspace-result workspace-result-message" key={`${reference.channelId}:${reference.messageId}`} onClick={() => channel && select(channel)}><span className="workspace-result-icon"><Icon name="bookmark" size={16} /></span><span><strong>{channel?.name}</strong><small>{message?.body}</small></span><time>{message?.time}</time></button>)}</div>
        </>}

        {kind === 'threads' && <>
          <p className="workspace-overlay-hint">{t('overlay.threadsHint')}</p>
          <div className="workspace-result-list">{threadItems.length === 0 && <div className="workspace-overlay-empty"><Icon name="thread" size={20} /><span>{t('overlay.noThreads')}</span></div>}{threadItems.map(({ channelId, message }) => { const channel = channelById.get(channelId); if (!channel) return null; return <button className="workspace-result workspace-result-message" key={`${channelId}:${message.id}`} onClick={() => { onOpenThread(channel, message); onClose() }}><span className="workspace-result-icon"><Icon name="thread" size={16} /></span><span><strong>{channel.name}</strong><small>{message.body}</small></span><span className="thread-count">{t('chat.replies', { count: message.threadCount ?? 0 })}</span></button> })}</div>
        </>}
      </div>
    </section>
  </div>
}
