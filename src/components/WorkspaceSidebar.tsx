import type { ApiUser } from '../services/chatApi'
import type { Channel } from '../types/chat'
import { Avatar, Icon } from './ChatIcons'
import { ProfileMenu } from './ProfileMenu'
import { useEffect, useRef, useState } from 'react'
import { t } from '../i18n'

type Props = {
  channels: Channel[]
  selectedChannelId: string
  currentUser: ApiUser
  myPresence: NonNullable<Channel['presence']>
  channelGroups: string[]
  onSelectChannel: (channel: Channel) => void
  onAddChannel: (group: string) => void
  onChangePresence: (presence: NonNullable<Channel['presence']>) => void
  onUpdateProfile: (name: string) => Promise<void>
  onLogout: () => void
  unreadCount: number
  savedCount: number
  threadCount: number
  onOpenSearch: () => void
  onOpenQuickLink: (kind: 'inbox' | 'saved' | 'threads') => void
  onOpenWorkspace: () => void
  onOpenHelp: () => void
}

export function WorkspaceSidebar({ channels, selectedChannelId, currentUser, myPresence, channelGroups, onSelectChannel, onAddChannel, onChangePresence, onUpdateProfile, onLogout, unreadCount, savedCount, threadCount, onOpenSearch, onOpenQuickLink, onOpenWorkspace, onOpenHelp }: Props) {
  const [profileOpen, setProfileOpen] = useState(false)
  const profileAreaRef = useRef<HTMLDivElement>(null)
  const groupLabel = (group: string) => t(`sidebar.groups.${group}`)

  useEffect(() => {
    if (!profileOpen) return
    const handlePointerDown = (event: PointerEvent) => {
      if (profileAreaRef.current && !profileAreaRef.current.contains(event.target as Node)) setProfileOpen(false)
    }
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setProfileOpen(false)
    }
    document.addEventListener('pointerdown', handlePointerDown)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('pointerdown', handlePointerDown)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [profileOpen])
  const channelButton = (channel: Channel) => <button key={channel.id} className={`channel-row ${channel.kind === 'dm' ? 'dm-row' : ''} ${selectedChannelId === channel.id ? 'channel-row-active' : ''}`} onClick={() => onSelectChannel(channel)}>{channel.kind === 'dm' ? <Avatar initials={channel.initials ?? ''} color={channel.color ?? '#394b6a'} presence={channel.presence} size="small" /> : <Icon name="hash" size={16} />}<span className="channel-name">{channel.name}</span>{channel.unread > 0 && <span className="unread-badge">{channel.unread}</span>}</button>

  return <>
    <aside className="workspace-rail"><button className="workspace-icon workspace-icon-active" onClick={onOpenWorkspace} aria-label={t('brand.workspaceName')}>LL</button><div className="rail-spacer" /><button className="rail-button" onClick={onOpenHelp} aria-label={t('sidebar.help')}><Icon name="help" size={19} /></button><button className="rail-avatar-button" onClick={() => setProfileOpen(true)} aria-label={t('profile.open')}><Avatar initials={currentUser.initials} color={currentUser.color} presence={myPresence} size="small" /></button></aside>
    <aside className="channel-sidebar">
      <div className="workspace-heading"><div><span className="eyebrow">{t('brand.workspaceEyebrow')}</span><h1>{t('brand.workspaceName')} <span className="verified-mark">✦</span></h1></div></div>
      <button className="workspace-search" onClick={onOpenSearch} aria-label={t('sidebar.jumpToConversation')}><Icon name="search" size={16} /><span>{t('sidebar.jumpToConversation')}</span><kbd>⌘ K</kbd></button>
      <nav className="quick-links" aria-label={t('sidebar.quickLinks')}><button className="quick-link" onClick={() => onOpenQuickLink('inbox')}><Icon name="inbox" size={17} /><span>{t('sidebar.inbox')}</span>{unreadCount > 0 && <span className="link-count">{unreadCount}</span>}</button><button className="quick-link" onClick={() => onOpenQuickLink('saved')}><Icon name="bookmark" size={17} /><span>{t('sidebar.saved')}</span>{savedCount > 0 && <span className="link-count">{savedCount}</span>}</button><button className="quick-link" onClick={() => onOpenQuickLink('threads')}><Icon name="thread" size={17} /><span>{t('sidebar.threads')}</span>{threadCount > 0 && <span className="link-count">{threadCount}</span>}</button></nav>
      <div className="channel-scroll">{channelGroups.map((group) => <section className="channel-group" key={group}><div className="group-heading"><span>{groupLabel(group)}</span><button className="tiny-button" onClick={() => onAddChannel(group)} aria-label={t('sidebar.addChannel', { group: groupLabel(group) })}><Icon name="plus" size={15} /></button></div>{channels.filter((channel) => channel.group === group).map(channelButton)}</section>)}<section className="channel-group"><div className="group-heading"><span>{t('sidebar.directMessages')}</span><button className="tiny-button" onClick={onOpenSearch} aria-label={t('sidebar.searchDirectMessages')} title={t('sidebar.searchDirectMessagesTitle')}><Icon name="search" size={15} /></button></div>{channels.filter((channel) => channel.group === 'Direct messages').map(channelButton)}</section></div>
      <div className="profile-area" ref={profileAreaRef}><button className="user-card" onClick={() => setProfileOpen((open) => !open)} aria-expanded={profileOpen} aria-label={t('profile.open')}><Avatar initials={currentUser.initials} color={currentUser.color} presence={myPresence} size="medium" /><span className="user-meta"><strong>{currentUser.name}</strong><small>{t(`profile.statuses.${myPresence}`)}</small></span><Icon name="settings" size={17} /></button>{profileOpen && <ProfileMenu user={currentUser} presence={myPresence} onChangePresence={onChangePresence} onUpdateProfile={onUpdateProfile} onClose={() => setProfileOpen(false)} />}</div><button className="logout-button" onClick={onLogout}>{t('sidebar.logout')}</button>
    </aside>
  </>
}
