/**
 * Builds a Track object from a catalog item, suitable for dispatching
 * via the 'play-track' CustomEvent.
 *
 * Runtime fields (position, duration, url, urlExpiresAt) are populated
 * by the Player after the track is loaded. They default to undefined
 * so the Player can detect "not yet loaded" vs "loaded but at zero".
 *
 * @param {any} item – a catalog item (any shape, fields are optional).
 * @returns {object} – a Track object compatible with the Player component.
 */
export function buildTrack(item) {
  const filename = item.filename ?? '';
  const title = item.title ?? filename;
  const speaker = item.speaker ?? '';
  return {
    id: item.id ?? filename,
    filename,
    hash: item.hash ?? '',
    playbackRate: 1.0,
    title,
    speaker,
    artist: speaker,
    metadata: {
      title,
      artist: speaker,
      speaker
    },
    // Runtime state – populated by Player on first play
    position: 0,
    duration: 0,
    url: '',
    urlExpiresAt: 0
  };
}
