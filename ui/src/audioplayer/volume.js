// The three volume representations must stay in sync (see AGENTS.md §3):
// the Redux store holds a perceived 0..1 value, the <audio> element takes its
// square, and remote outputs take round(store*100). Keeping the conversions here
// (instead of inline in Player.jsx) is what makes them unit-testable — a silent
// change of the square is exactly the kind of bug that only shows up as
// "the slider does nothing".

// store (0..1 perceived) -> <audio> element (0..1 physical)
export const perceivedToElementVolume = (perceived) => {
  const volume = Math.min(1, Math.max(0, perceived ?? 0))
  return volume * volume
}

// device-reported percent (0..100) -> store value, or null when it must be
// ignored. Drivers report 0 until the device answers its first volume query, and
// adopting that would silence the UI and persist the mute (the historical
// "volume turns to 0% after a refresh" bug).
export const devicePercentToPerceived = (percent) => {
  if (typeof percent !== 'number' || Number.isNaN(percent)) {
    return null
  }
  if (percent <= 0 || percent > 100) {
    return null
  }
  return percent / 100
}

// store (0..1) -> remote output percent (0..100)
export const perceivedToDevicePercent = (perceived) => {
  const volume = Math.min(1, Math.max(0, perceived ?? 0))
  return Math.round(volume * 100)
}

// Below this delta the two values are considered equal, so the element is not
// written on every volumechange ping-pong.
export const VOLUME_EPSILON = 0.005
