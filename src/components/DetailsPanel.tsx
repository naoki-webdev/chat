import type { Channel } from '../types/chat'
import type { ApiChannelSummary } from '../services/chatApi'
import { Avatar, Icon } from './ChatIcons'
import { t } from '../i18n'

type Member = { name: string; handle: string; initials: string; role: string; presence: NonNullable<Channel['presence']>; color: string }
type SummarySection = { title: string; items: ApiChannelSummary['decisions']; tone: 'decision' | 'action' | 'unresolved' }

export function DetailsPanel({ selectedChannel, members, summary, summaryLoading, summaryError, onGenerateSummary, onJumpToMessage, onClose }: { selectedChannel: Channel; members: Member[]; summary: ApiChannelSummary | null; summaryLoading: boolean; summaryError?: string; onGenerateSummary: () => void; onJumpToMessage: (messageId: string) => void; onClose: () => void }) {
  const isChannel = selectedChannel.kind === 'channel'
  const sections: SummarySection[] = summary ? [
    { title: t('details.summary.decisions'), items: summary.decisions, tone: 'decision' },
    { title: t('details.summary.actions'), items: summary.action_items, tone: 'action' },
    { title: t('details.summary.unresolved'), items: summary.unresolved, tone: 'unresolved' },
  ] : []

  return <aside className="details-panel">
    <div className="details-heading"><div><span className="eyebrow">{t('details.eyebrow')}</span><h2>{isChannel ? t('details.channelTitle') : t('details.conversationTitle')}</h2></div><button className="icon-button" onClick={onClose} aria-label={t('details.close')}>×</button></div>
    <div className="details-scroll">
      <div className="details-cover"><div className="cover-pattern" /><div className="details-channel-icon"><Icon name={isChannel ? 'hash' : 'message'} size={24} /></div><h3>{selectedChannel.name}</h3><p>{selectedChannel.description ?? t('details.directDescription')}</p></div>
      <section className="summary-section" aria-label={t('details.summary.ariaLabel')}>
        <div className="summary-heading"><div><span className="eyebrow">{t('details.summary.eyebrow')}</span><strong>{t('details.summary.heading')}</strong></div>{summary && summary.scope === 'unread' && <span className="summary-count summary-count-stack"><span>{t('details.summary.unreadCount', { count: summary.unread_count })}</span><span>{t('details.summary.unreadAnalyzedCount', { count: summary.message_count })}</span></span>}{summary && summary.scope !== 'unread' && <span className="summary-count">{t('details.summary.analyzedCount', { count: summary.message_count })}</span>}</div>
        {!summary && !summaryLoading && <p className="summary-empty">{t('details.summary.empty')}</p>}
        {summaryLoading && <p className="summary-loading"><span className="summary-spinner" />{t('details.summary.loading')}</p>}
        {summaryError && <p className="summary-error" role="alert">{summaryError}</p>}
        {summary && <>
          <p className="summary-lead">{summary.summary}</p>
          {sections.map((section) => <div className={`summary-group summary-group-${section.tone}`} key={section.title}>
            <div className="summary-group-title"><span>{section.title}</span><span>{section.items.length}</span></div>
            {section.items.length === 0 ? <p className="summary-none">{t('details.summary.none')}</p> : section.items.map((item, index) => item.source_message_id ? <button className="summary-item" key={`${item.source_message_id}-${index}`} onClick={() => onJumpToMessage(item.source_message_id ?? '')}><span>{item.text}</span><Icon name="chevron" size={13} /></button> : <p className="summary-item summary-item-static" key={`${item.text}-${index}`}><span>{item.text}</span></p>)}
          </div>)}
          <div className="summary-chatter"><span>{t('details.summary.chatterLabel')}</span><span>{t('details.summary.chatterCount', { count: summary.chatter_count })}</span></div>
        </>}
        <button className="summary-generate" onClick={onGenerateSummary} disabled={summaryLoading} aria-label={summary ? t('details.summary.update') : t('details.summary.summarize')}>{summaryLoading ? t('details.summary.loading') : summary ? t('details.summary.update') : t('details.summary.summarize')}<span>✦</span></button>
      </section>
      <div className="details-section"><div className="details-section-title"><span>{t('details.members')}</span><span className="member-count">{members.length}</span></div><div className="member-list">{members.map((member) => <div className="member-row" key={member.handle}><Avatar initials={member.initials} color={member.color} presence={member.presence} size="small" /><div><strong>{member.name}</strong><small>{member.role}</small></div></div>)}</div></div>
    </div>
    <div className="details-footer"><span className="status-check"><Icon name="check" size={14} /> {t('details.noUnread')}</span><span>{t('details.liveSync')}</span></div>
  </aside>
}
