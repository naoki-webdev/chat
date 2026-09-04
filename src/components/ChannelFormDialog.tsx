import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { Avatar, Icon } from './ChatIcons'
import { t } from '../i18n'
import type { ApiChannelMember, ApiMember } from '../services/chatApi'

type ChannelFormValues = {
  name: string
  description: string
  group?: string
  memberIds: string[]
}

type Props = {
  mode: 'create' | 'edit'
  initialGroup?: string
  groups?: string[]
  initialName?: string
  initialDescription?: string
  initialMemberIDs: string[]
  members: ApiMember[]
  currentUserId: string
  currentUserRole?: ApiChannelMember['role']
  onSubmit: (values: ChannelFormValues) => Promise<void>
  onClose: () => void
}

export function ChannelFormDialog({ mode, initialGroup = '', groups = [], initialName = '', initialDescription = '', initialMemberIDs, members, currentUserId, currentUserRole, onSubmit, onClose }: Props) {
  const [name, setName] = useState(initialName)
  const [group, setGroup] = useState(initialGroup)
  const [description, setDescription] = useState(initialDescription)
  const [selectedMemberIDs, setSelectedMemberIDs] = useState<string[]>(initialMemberIDs)
  const [saving, setSaving] = useState(false)
  const selectableMembers = useMemo(() => mode === 'create' ? members.filter((member) => member.id !== currentUserId) : members, [currentUserId, members, mode])
  const isCreate = mode === 'create'

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [onClose])

  useEffect(() => {
    if (!isCreate) setSelectedMemberIDs(initialMemberIDs)
  }, [initialMemberIDs, isCreate])

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const trimmedName = name.trim()
    if (!trimmedName) return
    setSaving(true)
    try {
      await onSubmit({ name: trimmedName, description: description.trim(), group: isCreate ? group : undefined, memberIds: selectedMemberIDs })
    } catch {
      // The parent shows the API error and keeps the dialog open.
    } finally {
      setSaving(false)
    }
  }

  const toggleMember = (memberId: string) => setSelectedMemberIDs((current) => current.includes(memberId) ? current.filter((id) => id !== memberId) : [...current, memberId])

  return <div className="workspace-overlay-backdrop channel-form-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
    <section className="workspace-overlay channel-form-dialog" role="dialog" aria-modal="true" aria-label={t(isCreate ? 'channel.createTitle' : 'channel.editTitle')}>
      <header className="workspace-overlay-header">
        <div><span className="eyebrow">{t(isCreate ? 'channel.createEyebrow' : 'channel.editEyebrow')}</span><h2>{t(isCreate ? 'channel.createTitle' : 'channel.editTitle')}</h2></div>
        <button className="icon-button" onClick={onClose} aria-label={t('overlay.close')}>×</button>
      </header>
      <div className="channel-form-body">
        <form onSubmit={submit}>
          <label className="channel-form-field"><span>{t('channel.nameLabel')}</span><input autoFocus value={name} maxLength={100} onChange={(event) => setName(event.target.value)} placeholder={isCreate ? t('channel.namePlaceholder') : undefined} /></label>
          {isCreate && <label className="channel-form-field"><span>{t('channel.groupLabel')}</span><div className="channel-select-wrap"><select value={group} onChange={(event) => setGroup(event.target.value)}>{groups.map((option) => <option value={option} key={option}>{t(`sidebar.groups.${option}`)}</option>)}</select><Icon name="chevron" size={14} /></div></label>}
          <label className="channel-form-field"><span>{t('channel.descriptionLabel')}</span><textarea value={description} maxLength={500} onChange={(event) => setDescription(event.target.value)} placeholder={isCreate ? t('channel.descriptionPlaceholder') : undefined} rows={3} /></label>
          <div className="channel-form-field"><div className="channel-field-label"><span>{t('channel.membersLabel')}</span><button className="inline-help" type="button" title={t('channel.membersHelp')} aria-label={t('channel.membersHelp')}>?</button></div><div className="channel-member-picker">{selectableMembers.map((member) => {
            const selected = selectedMemberIDs.includes(member.id)
            const locked = !isCreate && member.id === currentUserId
            return <label className={`channel-member-option ${selected ? 'channel-member-option-selected' : ''}`} key={member.id}><input type="checkbox" checked={selected} disabled={locked} onChange={() => toggleMember(member.id)} /><Avatar initials={member.initials} color={member.color} size="small" /><span><strong>{member.name}</strong><small>{locked ? t(`details.roles.${currentUserRole ?? 'member'}`) : `@${member.handle}`}</small></span></label>
          })}{selectableMembers.length === 0 && <p className="channel-member-empty">{t('channel.membersEmpty')}</p>}</div></div>
          <div className="channel-form-actions"><button type="button" className="channel-form-cancel" onClick={onClose}>{t('channel.cancel')}</button><button type="submit" className="channel-form-submit" disabled={saving || !name.trim()}>{saving ? t(isCreate ? 'channel.creating' : 'channel.saving') : t(isCreate ? 'channel.create' : 'channel.save')}</button></div>
        </form>
      </div>
    </section>
  </div>
}
