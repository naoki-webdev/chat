import type { ApiUser } from '../services/chatApi'
import type { Channel, Message } from './chat'

export const demoUser: ApiUser = {
  id: 'demo',
  name: 'Taro Tanaka',
  email: 'demo@example.com',
  handle: 'taro',
  initials: 'TT',
  color: 'linear-gradient(135deg, #f3a683, #c56cf0)',
}

export const initialChannels: Channel[] = [
  { id: 'general', name: 'general', group: 'Engineering', kind: 'channel', unread: 0, description: 'プロジェクトの最新情報と雑談' },
  { id: 'frontend', name: 'frontend', group: 'Engineering', kind: 'channel', unread: 3, description: 'フロントエンド開発の相談場所' },
  { id: 'design-system', name: 'design-system', group: 'Engineering', kind: 'channel', unread: 0, description: 'OrbitのUIとデザイントークン' },
  { id: 'roadmap', name: 'roadmap', group: 'Product', kind: 'channel', unread: 0, description: 'プロダクトの方向性を話す場所' },
  { id: 'research', name: 'user-research', group: 'Product', kind: 'channel', unread: 1, description: 'ユーザーインタビューと発見' },
  { id: 'ayaka', name: 'Ayaka Mori', group: 'Direct messages', kind: 'dm', unread: 0, presence: 'online', initials: 'AM', color: 'linear-gradient(135deg, #f8c291, #e55039)' },
  { id: 'ken', name: 'Ken Ito', group: 'Direct messages', kind: 'dm', unread: 2, presence: 'away', initials: 'KI', color: 'linear-gradient(135deg, #82ccdd, #60a3bc)' },
  { id: 'orbit-ai', name: 'Orbit AI', group: 'Direct messages', kind: 'dm', unread: 0, presence: 'online', initials: '✦', color: 'linear-gradient(135deg, #8b5cf6, #22d3ee)', description: 'リアルタイム会話に参加するAIアシスタント' },
]

export const initialMessages: Record<string, Message[]> = {
  'design-system': [
    { id: 'ds-1', author: 'Ayaka Mori', initials: 'AM', color: 'linear-gradient(135deg, #f8c291, #e55039)', time: '09:42', body: '新しいカラートークンをまとめました。primaryの彩度を少し下げて、長時間見ても疲れにくい色にしています。', reactions: [{ emoji: '✨', count: 4 }, { emoji: '👍', count: 2, reacted: true }], threadCount: 3 },
    { id: 'ds-2', author: 'Ken Ito', initials: 'KI', color: 'linear-gradient(135deg, #82ccdd, #60a3bc)', time: '09:47', body: 'いい感じです！ダークモードのコントラスト比も確認しておきます。', reactions: [{ emoji: '👀', count: 1 }] },
    { id: 'ds-3', author: 'Taro Tanaka', initials: 'TT', color: demoUser.color, time: '10:01', body: 'ありがとう。チャット画面にもそのまま適用してみました。カードの境界線だけ、もう少し薄くしてもよさそうです。', reactions: [{ emoji: '💡', count: 3 }], threadCount: 1 },
    { id: 'ds-4', author: 'Ayaka Mori', initials: 'AM', color: 'linear-gradient(135deg, #f8c291, #e55039)', time: '10:08', body: '賛成です。余白とタイポグラフィが主役になるくらいの薄さにしておきます。', reactions: [{ emoji: '❤️', count: 5, reacted: true }] },
    { id: 'ds-5', author: 'Ayaka Mori', initials: 'AM', color: 'linear-gradient(135deg, #f8c291, #e55039)', time: '10:09', body: 'あと、モバイル幅では右の詳細パネルをドロワーにする案も考えています。', reactions: [{ emoji: '📱', count: 2 }] },
  ],
  general: [
    { id: 'g-1', author: 'Ken Ito', initials: 'KI', color: 'linear-gradient(135deg, #82ccdd, #60a3bc)', time: '08:30', body: 'おはようございます。今週もよろしくお願いします！', reactions: [{ emoji: '☀️', count: 5 }] },
    { id: 'g-2', author: 'Taro Tanaka', initials: 'TT', color: demoUser.color, time: '08:33', body: 'おはよう！リアルタイムチャットの初期画面を作り始めます。', reactions: [{ emoji: '🚀', count: 2, reacted: true }] },
  ],
  frontend: [
    { id: 'f-1', author: 'Ayaka Mori', initials: 'AM', color: 'linear-gradient(135deg, #f8c291, #e55039)', time: '昨日', body: 'APIレスポンスの型定義、shared/typesに置いておくと使いやすそうです。', reactions: [{ emoji: '👍', count: 3 }], threadCount: 2 },
    { id: 'f-2', author: 'Ken Ito', initials: 'KI', color: 'linear-gradient(135deg, #82ccdd, #60a3bc)', time: '昨日', body: '了解です。Go APIのレスポンス型と名前を揃えます。', reactions: [{ emoji: '✅', count: 1 }] },
  ],
  roadmap: [{ id: 'r-1', author: 'Taro Tanaka', initials: 'TT', color: demoUser.color, time: '月曜日', body: 'P0は認証、ワークスペース、チャンネル、履歴取得、リアルタイム送受信で切りましょう。', reactions: [{ emoji: '🎯', count: 3 }] }],
  research: [{ id: 'ur-1', author: 'Ayaka Mori', initials: 'AM', color: 'linear-gradient(135deg, #f8c291, #e55039)', time: '火曜日', body: 'インタビューで「未読がどこにあるか分からない」という声が多かったので、左サイドバーに寄せたいです。', reactions: [{ emoji: '🔎', count: 2 }] }],
  ayaka: [{ id: 'a-1', author: 'Ayaka Mori', initials: 'AM', color: 'linear-gradient(135deg, #f8c291, #e55039)', time: '11:15', body: 'チャット画面の雰囲気、かなり良くなってきましたね。', reactions: [{ emoji: '✨', count: 1 }] }],
  ken: [{ id: 'k-1', author: 'Ken Ito', initials: 'KI', color: 'linear-gradient(135deg, #82ccdd, #60a3bc)', time: '昨日', body: '再接続時の差分取得、WebSocket再接続後に入れるのが良さそうです。', reactions: [{ emoji: '💻', count: 2 }] }],
  'orbit-ai': [],
}
