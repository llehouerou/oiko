// A camera's Live view (ADR 0036, 0037): fragmented MP4 in the answer to a POST, played through
// Media Source.

// The Media Source a browser has: MediaSource, or ManagedMediaSource where only that exists
// (Safari on iPhone, from iOS 17.1); undefined without either.
export function mediaSourceOf(w: { MediaSource?: typeof MediaSource; ManagedMediaSource?: typeof MediaSource }) {
  return w.MediaSource ?? w.ManagedMediaSource
}

// Whether a Live view that ended was stopped at its time, the Live-View-Until its answer told
// (a camera on battery), rather than by its camera: then it offers to keep watching.
export const timeUp = (until: string | null, now: number) => until !== null && now >= Date.parse(until) - 5000

// How long a Live view lasted, as its History marker tells it.
export function watchedFor(ms: number) {
  const s = Math.round(ms / 1000)
  return s < 60 ? `${s} s` : `${Math.round(s / 60)} min`
}

// What a viewer shows: loading until the first frame plays, then playing; once it ended, whether
// its time was up or it failed, with why; unsupported without Media Source.
export type Viewing = { kind: 'loading' } | { kind: 'playing' } | { kind: 'timeUp' } | { kind: 'failed'; why: string } | { kind: 'unsupported' }

// The data a SourceBuffer may still hold: the last keep seconds before the playhead, so a long
// Live view does not fill the browser's quota.
export const keep = 30

// How far behind the live edge playback may fall before it jumps to it, in seconds.
export const maxLag = 3
