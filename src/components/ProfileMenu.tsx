import { useEffect, useState, type FormEvent } from 'react'
import type { ApiUser } from '../services/chatApi'
import type { Channel } from '../types/chat'
import { t } from '../i18n'

type Presence = NonNullable<Channel['presence']>

type Props = {
  user: ApiUser
  presence: Presence
  onChangePresence: (presence: Presence) => void
  onUpdateProfile: (name: string) => Promise<void>
  onClose: () => void
}

const presenceOptions: Presence[] = ['online', 'away', 'offline']

export function ProfileMenu({ user, presence, onChangePresence, onUpdateProfile, onClose }: Props) {
  const [name, setName] = useState(user.name)
  const [saving, setSaving] = useState(false)

  useEffect(() => { setName(user.name) }, [user.name])

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const nextName = name.trim()
    if (!nextName || nextName === user.name) {
      onClose()
      return
    }
    setSaving(true)
    try {
      await onUpdateProfile(nextName)
      onClose()
    } catch {
      // The parent displays the API error and keeps the menu open for another attempt.
    } finally {
      setSaving(false)
    }
  }

  return <section className="profile-menu" role="dialog" aria-label={t('profile.title')}>
    <div className="profile-menu-heading"><strong>{t('profile.title')}</strong><button type="button" className="profile-menu-close" onClick={onClose} aria-label={t('profile.close')}>×</button></div>
    <form onSubmit={submit}>
      <label className="profile-field"><span>{t('profile.displayName')}</span><input value={name} maxLength={80} onChange={(event) => setName(event.target.value)} /></label>
      <div className="profile-field"><span>{t('profile.status')}</span><div className="profile-status-list">{presenceOptions.map((option) => <button type="button" key={option} className={`profile-status-option ${presence === option ? 'profile-status-option-active' : ''}`} aria-pressed={presence === option} onClick={() => onChangePresence(option)}><span className={`profile-status-dot profile-status-dot-${option}`} />{t(`profile.statuses.${option}`)}</button>)}</div></div>
      <button type="submit" className="profile-save" disabled={saving || !name.trim()}>{saving ? t('profile.saving') : t('profile.save')}</button>
    </form>
  </section>
}
