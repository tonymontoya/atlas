export function formatDate(value: string): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(parsed);
}

// CoarseDuration is the shared bucketing behind recency ("3h ago") and
// time-until ("expires in 3h") labels: one vocabulary of units, callers
// keep their own phrasing and edge cases.
export type CoarseDuration = {
  seconds: number;
  minutes: number;
  hours: number;
  days: number;
};

export function coarseDuration(ms: number): CoarseDuration {
  const seconds = Math.floor(ms / 1000);
  return {
    seconds,
    minutes: Math.floor(seconds / 60),
    hours: Math.floor(seconds / 3600),
    days: Math.floor(seconds / 86400),
  };
}

export function errorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return "Request failed.";
}
