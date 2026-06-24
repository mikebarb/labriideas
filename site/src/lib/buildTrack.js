/**
 * Builds a Track object from a catalog item, suitable for dispatching
 * via the 'play-track' CustomEvent.
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
    }
  };
}
