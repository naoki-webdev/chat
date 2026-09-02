import { useEffect, useState, type FormEvent } from 'react'
import { Avatar, Icon } from './ChatIcons'
import { t } from '../i18n'
import type { ApiMember } from '../services/chatApi'

type Props = {
  initialGroup: string
  groups: string[]
  members: ApiMember[]
  currentUserId: string
  onCreate: (payload: { name: string; group: string; description: string; memberIds: string[] }) => Promise<void>
  onClose: () => void
}

export function ChannelCreateDialog({ initialGroup, groups, members, currentUserId, onCreate, onClose }: Props) {
  const [name, setName] = useState('')
  const [group, setGroup] = useState(initialGroup)
  const [description, setDescription] = useState('')
  const [selectedMemberIDs, setSelectedMemberIDs] = useState<string[]>([])
  const [saving, setSaving] = useState(false)

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
      await onCreate({ name: trimmedName, group, description: description.trim(), memberIds: selectedMemberIDs })
    } catch {
      // The parent shows the API error and keeps the dialog open.
    } finally {
      setSaving(false)
    }
  }

  return <div className="workspace-overlay-backdrop channel-create-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
    <section className="workspace-overlay channel-create-dialog" role="dialog" aria-modal="true" aria-label={t('channel.createTitle')}>
      <header className="workspace-overlay-header">
        <div><span className="eyebrow">{t('channel.createEyebrow')}</span><h2>{t('channel.createTitle')}</h2></div>
        <button className="icon-button" onClick={onClose} aria-label={t('overlay.close')}>×</button>
      </header>
      <div className="channel-create-body">
        <form onSubmit={submit}>
          <label className="channel-create-field"><span>{t('channel.nameLabel')}</span><input autoFocus value={name} maxLength={100} onChange={(event) => setName(event.target.value)} placeholder={t('channel.namePlaceholder')} /></label>
          <label className="channel-create-field"><span>{t('channel.groupLabel')}</span><div className="channel-select-wrap"><select value={group} onChange={(event) => setGroup(event.target.value)}>{groups.map((option) => <option value={option} key={option}>{t(`sidebar.groups.${option}`)}</option>)}</select><Icon name="chevron" size={14} /></div></label>
          <label className="channel-create-field"><span>{t('channel.descriptionLabel')}</span><textarea value={description} maxLength={500} onChange={(event) => setDescription(event.target.value)} placeholder={t('channel.descriptionPlaceholder')} rows={3} /></label>
          <div className="channel-create-field"><div className="channel-field-label"><span>{t('channel.membersLabel')}</span><button className="inline-help" type="button" title={t('channel.membersHelp')} aria-label={t('channel.membersHelp')}>?</button></div><div className="channel-member-picker">{members.filter((member) => member.id !== currentUserId).map((member) => {
            const selected = selectedMemberIDs.includes(member.id)
            return <label className={`channel-member-option ${selected ? 'channel-member-option-selected' : ''}`} key={member.id}><input type="checkbox" checked={selected} onChange={() => setSelectedMemberIDs((current) => selected ? current.filter((id) => id !== member.id) : [...current, member.id])} /><Avatar initials={member.initials} color={member.color} size="small" /><span><strong>{member.name}</strong><small>@{member.handle}</small></span></label>
          })}{members.filter((member) => member.id !== currentUserId).length === 0 && <p className="channel-member-empty">{t('channel.membersEmpty')}</p>}</div></div>
          <div className="channel-create-actions"><button type="button" className="channel-create-cancel" onClick={onClose}>{t('channel.cancel')}</button><button type="submit" className="channel-create-submit" disabled={saving || !name.trim()}>{saving ? t('channel.creating') : t('channel.create')}</button></div>
        </form>
      </div>
    </section>
  </div>
}
