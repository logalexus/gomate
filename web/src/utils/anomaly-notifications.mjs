export function notificationScore(value) {
 return Number.isInteger(value) && value >= 1 && value <= 1000 ? value : 40;
}

// Coalesce updates within one flush before marking sessions as delivered.
export function eligibleNotices(items, threshold, seen) {
 const sessions = new Map();
 for (const item of items) {
  if (!Number.isSafeInteger(item.streamId) || item.streamId < 1 || !Number.isInteger(item.service) ||
      item.service < 1 || item.service > 65535 || !Number.isFinite(item.score) || item.score < threshold || seen.has(item.streamId)) continue;
  if (!sessions.has(item.streamId) || sessions.get(item.streamId).score < item.score) sessions.set(item.streamId, item);
 }
 return [...sessions.values()].sort((a,b) => b.score-a.score);
}
