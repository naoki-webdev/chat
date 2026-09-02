import { expect, type Page } from '@playwright/test'

export async function login(page: Page, email = 'demo@example.com') {
  await page.goto('/')
  if (email === 'demo@example.com') {
    await page.getByRole('button', { name: 'ログイン' }).click()
  } else {
    await page.getByLabel('メールアドレス').fill(email)
    await page.getByLabel('パスワード').fill('demo-password')
    await page.getByRole('button', { name: 'ログイン' }).click()
  }
  await expect(page.getByText('Lumen Labs')).toBeVisible()
}

