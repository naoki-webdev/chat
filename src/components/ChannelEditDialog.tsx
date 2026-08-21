import { useEffect, useState, type FormEvent } from 'react'
import { Avatar } from './ChatIcons'
import type { ApiChannelMember, ApiMember } from '../services/chatApi'
import type { Channel } from '../types/chat'
import { t } from '../i18n'

type Props = {
  channel: Channel
  members: ApiMember[]
  channelMembers: ApiChannelMember[]
  currentUserId: string
  currentUserRole: ApiChannelMember['role']
  onSave: (payload: { name: string; description: string; memberIds: string[] }) => Promise<void>
  onClose: () => void
}

export function ChannelEditDialog({ channel, members, channelMembers, currentUserId, currentUserRole, onSave, onClose }: Props) {
  const [name, setName] = useState(channel.name)
  const [description, setDescription] = useState(channel.description ?? '')
  const memberIDsForSave = () => Array.from(new Set([currentUserId, ...channelMembers.filter((member) => !member.is_bot).map((member) => member.id)]))
  const [selectedMemberIDs, setSelectedMemberIDs] = useState<string[]>(memberIDsForSave)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    setSelectedMemberIDs(memberIDsForSave())
  }, [channelMembers, currentUserId])

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [onClose])

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const trimmedName = name.trim()
    if (!trimmedName) return
    setSaving(true)
    try {
      await onSave({ name: trimmedName, description: description.trim(), memberIds: selectedMemberIDs })
    } catch {
      // The parent shows the API error and keeps the dialog open.
    } finally {
      setSaving(false)
    }
  }

  return <div className="workspace-overlay-backdrop channel-edit-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
    <section className="workspace-overlay channel-edit-dialog" role="dialog" aria-modal="true" aria-label={t('channel.editTitle')}>
      <header className="workspace-overlay-header">
        <div><span className="eyebrow">{t('channel.editEyebrow')}</span><h2>{t('channel.editTitle')}</h2></div>
        <button className="icon-button" onClick={onClose} aria-label={t('overlay.close')}>×</button>
      </header>
      <div className="channel-create-body">
        <form onSubmit={submit}>
          <label className="channel-create-field"><span>{t('channel.nameLabel')}</span><input autoFocus value={name} maxLength={100} onChange={(event) => setName(event.target.value)} /></label>
          <label className="channel-create-field"><span>{t('channel.descriptionLabel')}</span><textarea value={description} maxLength={500} onChange={(event) => setDescription(event.target.value)} rows={3} /></label>
          <div className="channel-create-field"><span>{t('channel.membersLabel')}</span><div className="channel-member-picker">{members.map((member) => {
            const selected = selectedMemberIDs.includes(member.id)
            const locked = member.id === currentUserId
            return <label className={`channel-member-option ${selected ? 'channel-member-option-selected' : ''}`} key={member.id}><input type="checkbox" checked={selected} disabled={locked} onChange={() => setSelectedMemberIDs((current) => selected ? current.filter((id) => id !== member.id) : [...current, member.id])} /><Avatar initials={member.initials} color={member.color} size="small" /><span><strong>{member.name}</strong><small>{locked ? t(`details.roles.${currentUserRole}`) : `@${member.handle}`}</small></span></label>
          })}{members.length === 0 && <p className="channel-member-empty">{t('channel.membersEmpty')}</p>}</div><small className="channel-create-hint">{t('channel.editMembersHint')}</small></div>
          <div className="channel-create-actions"><button type="button" className="channel-create-cancel" onClick={onClose}>{t('channel.cancel')}</button><button type="submit" className="channel-create-submit" disabled={saving || !name.trim()}>{saving ? t('channel.saving') : t('channel.save')}</button></div>
        </form>
      </div>
    </section>
  </div>
}
