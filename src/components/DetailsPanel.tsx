import type { Channel } from '../types/chat'
import type { ApiChannelSummary } from '../services/chatApi'
import { Avatar, Icon } from './ChatIcons'
import { t } from '../i18n'

type Member = { name: string; handle: string; initials: string; role: string; presence: NonNullable<Channel['presence']>; color: string }
type SummarySection = { title: string; items: ApiChannelSummary['decisions']; tone: 'decision' | 'action' | 'unresolved' }

export function DetailsPanel({ selectedChannel, members, summary, summaryLoading, summaryError, connection, onGenerateSummary, onJumpToMessage, onClose }: { selectedChannel: Channel; members: Member[]; summary: ApiChannelSummary | null; summaryLoading: boolean; summaryError?: string; connection: 'connected' | 'reconnecting'; onGenerateSummary: () => void; onJumpToMessage: (messageId: string, parentMessageId?: string) => void; onClose: () => void }) {
  const isChannel = selectedChannel.kind === 'channel'
  const sections: SummarySection[] = summary ? [
    { title: t('details.summary.decisions'), items: summary.decisions, tone: 'decision' },
    { title: t('details.summary.actions'), items: summary.action_items, tone: 'action' },
    { title: t('details.summary.unresolved'), items: summary.unresolved, tone: 'unresolved' },
  ] : []

  return <aside className="details-panel">
    <div className="details-heading"><div className="details-heading-channel"><div className="details-channel-icon"><Icon name={isChannel ? 'hash' : 'message'} size={18} /></div><div><h2>{selectedChannel.name}</h2><p>{selectedChannel.description ?? t('details.directDescription')}</p></div></div><button className="icon-button" onClick={onClose} aria-label={t('details.close')}>×</button></div>
    <div className="details-scroll">
      <section className="summary-section" aria-label={t('details.summary.ariaLabel')}>
        <div className="summary-heading"><div className="summary-heading-title"><strong>{t('details.summary.heading')}</strong><button className="inline-help" type="button" title={t('details.summary.help')} aria-label={t('details.summary.help')}>?</button></div>{summary?.scope === 'unread' && <span className="summary-count" title={t('details.summary.unreadAnalyzedCount', { count: summary.message_count })}>{t('details.summary.unreadCount', { count: summary.unread_count })}</span>}</div>
        {summaryLoading && <p className="summary-loading"><span className="summary-spinner" />{t('details.summary.loading')}</p>}
        {summaryError && <p className="summary-error" role="alert">{summaryError}</p>}
        {summary && <>
          <p className="summary-lead">{summary.summary}</p>
          {sections.map((section) => <div className={`summary-group summary-group-${section.tone}`} key={section.title}>
            <div className="summary-group-title"><span>{section.title}</span><span>{section.items.length}</span></div>
            {section.items.length === 0 ? <p className="summary-none">{t('details.summary.none')}</p> : section.items.map((item, index) => item.source_message_id ? <button className="summary-item" key={`${item.source_message_id}-${index}`} onClick={() => onJumpToMessage(item.source_message_id ?? '', item.source_parent_message_id)}><span>{item.text}</span><Icon name="chevron" size={13} /></button> : <p className="summary-item summary-item-static" key={`${item.text}-${index}`}><span>{item.text}</span></p>)}
          </div>)}
          <div className="summary-chatter"><span>{t('details.summary.chatterLabel')}</span><span>{t('details.summary.chatterCount', { count: summary.chatter_count })}</span></div>
        </>}
        <button className="summary-generate" onClick={onGenerateSummary} disabled={summaryLoading} aria-label={summary ? t('details.summary.update') : t('details.summary.summarize')}>{summaryLoading ? t('details.summary.loading') : summary ? t('details.summary.update') : t('details.summary.summarize')}<span>✦</span></button>
      </section>
      <div className="details-section"><div className="details-section-title"><span>{t('details.members')}</span><span className="member-count">{members.length}</span></div><div className="member-list">{members.length === 0 ? <p className="details-empty">{t('details.membersEmpty')}</p> : members.map((member) => <div className="member-row" key={member.handle}><Avatar initials={member.initials} color={member.color} presence={member.presence} size="small" /><div><strong>{member.name}</strong><small>{member.role}</small></div></div>)}</div></div>
    </div>
    <div className="details-footer"><span className={`status-live status-live-${connection}`} title={t('chat.connectionTitle')} aria-label={t('chat.connectionTitle')}><span aria-hidden="true">●</span> {t(`chat.${connection}`)}</span></div>
  </aside>
}
