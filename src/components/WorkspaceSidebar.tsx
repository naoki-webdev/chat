import type { ApiUser } from '../services/chatApi'
import type { Channel } from '../types/chat'
import { Avatar, Icon } from './ChatIcons'
import { t } from '../i18n'

type Props = {
  channels: Channel[]
  selectedChannelId: string
  currentUser: ApiUser
  myPresence: NonNullable<Channel['presence']>
  channelGroups: string[]
  onSelectChannel: (channel: Channel) => void
  onAddChannel: () => void
  onTogglePresence: () => void
  onLogout: () => void
  unreadCount: number
  savedCount: number
  threadCount: number
  onOpenSearch: () => void
  onOpenQuickLink: (kind: 'inbox' | 'saved' | 'threads') => void
  onOpenWorkspace: () => void
  onOpenHelp: () => void
  onHome: () => void
}

export function WorkspaceSidebar({ channels, selectedChannelId, currentUser, myPresence, channelGroups, onSelectChannel, onAddChannel, onTogglePresence, onLogout, unreadCount, savedCount, threadCount, onOpenSearch, onOpenQuickLink, onOpenWorkspace, onOpenHelp, onHome }: Props) {
  const groupLabel = (group: string) => t(`sidebar.groups.${group}`)
  const channelButton = (channel: Channel) => <button key={channel.id} className={`channel-row ${channel.kind === 'dm' ? 'dm-row' : ''} ${selectedChannelId === channel.id ? 'channel-row-active' : ''}`} onClick={() => onSelectChannel(channel)}>{channel.kind === 'dm' ? <Avatar initials={channel.initials ?? ''} color={channel.color ?? '#394b6a'} presence={channel.presence} size="small" /> : <Icon name="hash" size={16} />}<span className="channel-name">{channel.name}</span>{channel.unread > 0 && <span className="unread-badge">{channel.unread}</span>}</button>

  return <>
    <aside className="workspace-rail"><button className="orbit-mark" onClick={onHome} aria-label={t('sidebar.home')}>O</button><div className="rail-separator" /><button className="workspace-icon workspace-icon-active" onClick={onOpenWorkspace} aria-label={t('brand.workspaceName')}>LL</button><button className="workspace-icon workspace-icon-muted" disabled aria-label={t('sidebar.addWorkspace')} title={t('sidebar.addWorkspaceTitle')}><Icon name="plus" size={19} /></button><div className="rail-spacer" /><button className="rail-button" onClick={onOpenHelp} aria-label={t('sidebar.help')}><Icon name="help" size={19} /></button><Avatar initials={currentUser.initials} color={currentUser.color} presence={myPresence} size="small" /></aside>
    <aside className="channel-sidebar">
      <div className="workspace-heading"><div><span className="eyebrow">{t('brand.workspaceEyebrow')}</span><h1>{t('brand.workspaceName')} <span className="verified-mark">✦</span></h1></div><button className="icon-button" onClick={onOpenWorkspace} aria-label={t('sidebar.settings')}><Icon name="chevron" size={17} /></button></div>
      <button className="workspace-search" onClick={onOpenSearch} aria-label={t('sidebar.jumpToConversation')}><Icon name="search" size={16} /><span>{t('sidebar.jumpToConversation')}</span><kbd>⌘ K</kbd></button>
      <nav className="quick-links" aria-label={t('sidebar.quickLinks')}><button className="quick-link" onClick={() => onOpenQuickLink('inbox')}><Icon name="inbox" size={17} /><span>{t('sidebar.inbox')}</span>{unreadCount > 0 && <span className="link-count">{unreadCount}</span>}</button><button className="quick-link" onClick={() => onOpenQuickLink('saved')}><Icon name="bookmark" size={17} /><span>{t('sidebar.saved')}</span>{savedCount > 0 && <span className="link-count">{savedCount}</span>}</button><button className="quick-link" onClick={() => onOpenQuickLink('threads')}><Icon name="thread" size={17} /><span>{t('sidebar.threads')}</span>{threadCount > 0 && <span className="link-count">{threadCount}</span>}</button></nav>
      <div className="channel-scroll">{channelGroups.map((group) => <section className="channel-group" key={group}><div className="group-heading"><span>{groupLabel(group)}</span><button className="tiny-button" onClick={onAddChannel} aria-label={t('sidebar.addChannel', { group: groupLabel(group) })}><Icon name="plus" size={15} /></button></div>{channels.filter((channel) => channel.group === group).map(channelButton)}</section>)}<section className="channel-group"><div className="group-heading"><span>{t('sidebar.directMessages')}</span><button className="tiny-button" onClick={onOpenSearch} aria-label={t('sidebar.searchDirectMessages')} title={t('sidebar.searchDirectMessagesTitle')}><Icon name="search" size={15} /></button></div>{channels.filter((channel) => channel.group === 'Direct messages').map(channelButton)}</section></div>
      <button className="user-card" onClick={onTogglePresence}><Avatar initials={currentUser.initials} color={currentUser.color} presence={myPresence} size="medium" /><span className="user-meta"><strong>{currentUser.name}</strong><small>{myPresence === 'online' ? t('sidebar.presence.online') : t('sidebar.presence.away')}</small></span><Icon name="settings" size={17} /></button><button className="logout-button" onClick={onLogout}>{t('sidebar.logout')}</button>
    </aside>
  </>
}
