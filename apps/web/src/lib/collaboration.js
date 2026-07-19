import { apiRequest } from './api'

function recordQuery(entityType, entityId) {
  return new URLSearchParams({ entityType, entityId: String(entityId) }).toString()
}

export async function getRecordFollowers({ entityType, entityId, signal } = {}) {
  const payload = await apiRequest(`/api/record-followers?${recordQuery(entityType, entityId)}`, {
    fallbackMessage: 'Unable to load record followers.',
    signal
  })
  return {
    entityType,
    entityId,
    ...(payload?.data || {}),
    following: Boolean(payload?.data?.following),
    followers: payload?.data?.followers || []
  }
}

export async function setRecordFollowing({ entityType, entityId, following, signal } = {}) {
  const payload = await apiRequest(`/api/record-followers/me?${recordQuery(entityType, entityId)}`, {
    method: following ? 'PUT' : 'DELETE',
    fallbackMessage: following ? 'Unable to follow this record.' : 'Unable to unfollow this record.',
    signal
  })
  return {
    entityType,
    entityId,
    ...(payload?.data || {}),
    following: Boolean(payload?.data?.following),
    followers: payload?.data?.followers || []
  }
}

export async function getActivityDigest({ scope = 'following', days = 7, actorUserId = 0, signal } = {}) {
  const query = new URLSearchParams({ scope, days: String(days) })
  if (actorUserId) query.set('actorUserId', String(actorUserId))
  const payload = await apiRequest(`/api/collaboration/activity-digest?${query}`, {
    fallbackMessage: 'Unable to load the activity digest.',
    signal
  })
  return payload?.data || { scope, days, totalActivities: 0, activeRecords: 0, activePeople: 0, activities: [] }
}
