import { BookmarkCheckIcon, BookmarkPlusIcon } from 'lucide-react'

import { useAddSubscription, useRemoveSubscription, useSubscription } from '@/api/subscriptions'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'

// Follows a library actor so the daily check records their new works; a second
// click unfollows. Failures surface through the mutation error toast.
export function FollowButton({ actorKey }: { actorKey: string }) {
  const { subscription, isPending } = useSubscription('actor', actorKey)
  const follow = useAddSubscription()
  const unfollow = useRemoveSubscription()
  const busy = isPending || follow.isPending || unfollow.isPending
  const icon = busy ? <Spinner /> : subscription ? <BookmarkCheckIcon /> : <BookmarkPlusIcon />

  return (
    <Button
      type="button"
      size="sm"
      variant={subscription ? 'secondary' : 'default'}
      disabled={busy}
      onClick={() => (subscription ? unfollow.mutate(subscription) : follow.mutate(actorKey))}
    >
      {icon}
      {subscription ? '已关注' : '关注'}
    </Button>
  )
}
