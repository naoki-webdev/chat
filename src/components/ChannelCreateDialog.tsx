import type { ApiMember } from '../services/chatApi'
import { ChannelFormDialog } from './ChannelFormDialog'

type Props = {
  initialGroup: string
  groups: string[]
  members: ApiMember[]
  currentUserId: string
  onCreate: (payload: { name: string; group: string; description: string; memberIds: string[] }) => Promise<void>
  onClose: () => void
}

export function ChannelCreateDialog({ initialGroup, groups, members, currentUserId, onCreate, onClose }: Props) {
  return <ChannelFormDialog mode="create" initialGroup={initialGroup} groups={groups} initialMemberIDs={[]} members={members} currentUserId={currentUserId} onSubmit={(values) => onCreate({ name: values.name, group: values.group ?? initialGroup, description: values.description, memberIds: values.memberIds })} onClose={onClose} />
}
