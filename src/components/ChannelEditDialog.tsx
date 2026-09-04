import { useMemo } from 'react'
import type { ApiChannelMember, ApiMember } from '../services/chatApi'
import type { Channel } from '../types/chat'
import { ChannelFormDialog } from './ChannelFormDialog'

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
  const initialMemberIDs = useMemo(() => Array.from(new Set([currentUserId, ...channelMembers.filter((member) => !member.is_bot).map((member) => member.id)])), [channelMembers, currentUserId])
  return <ChannelFormDialog mode="edit" initialName={channel.name} initialDescription={channel.description ?? ''} initialMemberIDs={initialMemberIDs} members={members} currentUserId={currentUserId} currentUserRole={currentUserRole} onSubmit={({ name, description, memberIds }) => onSave({ name, description, memberIds })} onClose={onClose} />
}
