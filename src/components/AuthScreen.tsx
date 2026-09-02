import { useState } from 'react'
import { ChatApiError, chatApi, type ApiUser } from '../services/chatApi'
import { t } from '../i18n'

export function AuthScreen({ onAuthenticated }: { onAuthenticated: (user: ApiUser) => void }) {
  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [name, setName] = useState('')
  const [email, setEmail] = useState('demo@example.com')
  const [password, setPassword] = useState('demo-password')
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setError('')
    setSubmitting(true)
    try {
      const user = mode === 'login' ? await chatApi.login({ email, password }) : await chatApi.register({ name, email, password })
      onAuthenticated(user)
    } catch (caught) {
      setError(caught instanceof ChatApiError ? caught.message : t('errors.apiUnavailable'))
    } finally {
      setSubmitting(false)
    }
  }

  return <main className="auth-shell"><section className="auth-card"><div className="orbit-mark auth-mark">O</div><span className="eyebrow">{t('brand.authEyebrow')}</span><h1>{t(mode === 'login' ? 'auth.loginTitle' : 'auth.registerTitle')}</h1><form onSubmit={submit} className="auth-form">{mode === 'register' && <label>{t('auth.name')}<input value={name} onChange={(event) => setName(event.target.value)} placeholder={t('auth.namePlaceholder')} required /></label>}<label>{t('auth.email')}<input type="email" value={email} onChange={(event) => setEmail(event.target.value)} placeholder={t('auth.emailPlaceholder')} required /></label><label>{t('auth.password')}<input type="password" value={password} onChange={(event) => setPassword(event.target.value)} minLength={8} required /></label>{error && <p className="auth-error" role="alert">{error}</p>}<button className="auth-submit" type="submit" disabled={submitting}>{submitting ? t('auth.connecting') : t(mode === 'login' ? 'auth.submitLogin' : 'auth.submitRegister')}</button></form><button className="auth-switch" onClick={() => { setMode(mode === 'login' ? 'register' : 'login'); setError('') }}>{t(mode === 'login' ? 'auth.switchToRegister' : 'auth.switchToLogin')}</button>{mode === 'login' && <small className="auth-demo-hint">{t('auth.demo')}</small>}</section></main>
}
